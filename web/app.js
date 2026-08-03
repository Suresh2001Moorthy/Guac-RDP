'use strict';

// === Protocol Constants ===
const MSG_FRAME_BATCH    = 0x01;
const MSG_DESKTOP_RESIZE = 0x02;
const MSG_MOUSE          = 0x03;
const MSG_KEYBOARD       = 0x04;
const MSG_CURSOR_SHAPE   = 0x05;
const MSG_CLIPBOARD      = 0x06;

// RDP Mouse Flags
const PTR_FLAGS_MOVE    = 0x0800;
const PTR_FLAGS_DOWN    = 0x8000;
const PTR_FLAGS_BUTTON1 = 0x1000; // Left
const PTR_FLAGS_BUTTON2 = 0x2000; // Right
const PTR_FLAGS_BUTTON3 = 0x4000; // Middle
const PTR_FLAGS_WHEEL   = 0x0200;
const PTR_FLAGS_WHEEL_NEGATIVE = 0x0100;

// RDP Keyboard Flags
const KBD_FLAGS_DOWN     = 0x4000;
const KBD_FLAGS_RELEASE  = 0x8000;
const KBD_FLAGS_EXTENDED = 0x0100;

// === Status Display ===
const statusEl = document.getElementById('status');
let statusTimeout = null;

function setStatus(msg, type, persist) {
    statusEl.textContent = msg;
    statusEl.className = 'visible' + (type ? ' ' + type : '');
    clearTimeout(statusTimeout);
    if (!persist) {
        statusTimeout = setTimeout(() => statusEl.className = '', 2000);
    }
}

// === Canvas Setup ===
const canvas = document.getElementById('rdp-canvas');
const rdpContainer = document.getElementById('rdp-container');
const loginOverlay = document.getElementById('login-overlay');
const loginForm = document.getElementById('login-form');
const ctx = canvas.getContext('2d', { alpha: false, willReadFrequently: false });

let rdpWidth = Math.max(1, Math.floor(window.innerWidth));
let rdpHeight = Math.max(1, Math.floor(window.innerHeight));
let fitMode = 'pixel'; // Default to pixel-perfect scaling if requested

// Set initial canvas size
canvas.width = rdpWidth;
canvas.height = rdpHeight;

function fitCanvas() {
    const scale = fitMode === 'contain'
        ? Math.min(window.innerWidth / rdpWidth, window.innerHeight / rdpHeight, 1)
        : 1;
    
    const cssWidth = Math.min(window.innerWidth, rdpWidth * scale);
    const cssHeight = Math.min(window.innerHeight, rdpHeight * scale);
    
    canvas.style.width = cssWidth + 'px';
    canvas.style.height = cssHeight + 'px';
    canvas.style.position = 'absolute';
    canvas.style.left = '0px';
    canvas.style.top = '0px';
    
    canvas.style.imageRendering = 'pixelated';
}
fitCanvas();
window.addEventListener('resize', fitCanvas);

// === WebSocket Connection ===
let ws = null;
let frameCount = 0;
let pendingFrames = [];
let renderScheduled = false;

function renderPendingFrames() {
    for (const frame of pendingFrames) {
        ctx.putImageData(frame.imageData, frame.x, frame.y);
    }
    pendingFrames = [];
    renderScheduled = false;
}

function connectRDP(host, user, pass) {
    setStatus('Connecting...', null, true);
    
    // Update resolution to current window size before connecting
    rdpWidth = Math.max(1, Math.floor(window.innerWidth));
    rdpHeight = Math.max(1, Math.floor(window.innerHeight));
    canvas.width = rdpWidth;
    canvas.height = rdpHeight;
    fitCanvas();

    const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProtocol}//${location.host}/ws?host=${encodeURIComponent(host)}&user=${encodeURIComponent(user)}&pass=${encodeURIComponent(pass)}&w=${rdpWidth}&h=${rdpHeight}`;

    ws = new WebSocket(wsUrl);
    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
        setStatus('Connected, waiting for first frame...', 'success', true);
        loginOverlay.style.display = 'none';
        rdpContainer.style.display = 'block';
        canvas.focus();
    };
    
    ws.onerror = () => {
        setStatus('Connection error', 'error', true);
        showLogin();
    };
    
    ws.onclose = () => {
        setStatus('Disconnected', 'error', true);
        showLogin();
    };

    ws.onmessage = handleWebSocketMessage;
}

function showLogin() {
    loginOverlay.style.display = 'flex';
    rdpContainer.style.display = 'none';
    ws = null;
}

loginForm.addEventListener('submit', (e) => {
    e.preventDefault();
    const host = document.getElementById('hostInput').value.trim();
    const user = document.getElementById('userInput').value.trim();
    const pass = document.getElementById('passInput').value;
    
    if (host && user && pass) {
        connectRDP(host, user, pass);
    }
});

function handleWebSocketMessage(event) {
    const data = new DataView(event.data);
    const msgType = data.getUint8(0);

    switch (msgType) {
        case MSG_FRAME_BATCH: {
            const rectCount = data.getUint16(1, false);
            let offset = 3;

            for (let i = 0; i < rectCount; i++) {
                const x = data.getUint16(offset, false);
                const y = data.getUint16(offset + 2, false);
                const w = data.getUint16(offset + 4, false);
                const h = data.getUint16(offset + 6, false);
                offset += 8;

                const pixelCount = w * h;
                const pixelBytes = pixelCount * 4;

                // BGRA → RGBA swap
                const pixels = new Uint8ClampedArray(event.data, offset, pixelBytes);
                for (let j = 0; j < pixelBytes; j += 4) {
                    const temp = pixels[j];
                    pixels[j] = pixels[j + 2];
                    pixels[j + 2] = temp;
                }

                const imageData = new ImageData(pixels, w, h);
                pendingFrames.push({ imageData, x, y });

                offset += pixelBytes;
            }

            if (!renderScheduled && pendingFrames.length > 0) {
                renderScheduled = true;
                requestAnimationFrame(renderPendingFrames);
            }

            frameCount++;
            if (frameCount === 1) {
                setStatus('Desktop ready', 'success');
            }
            break;
        }

        case MSG_DESKTOP_RESIZE: {
            const newW = data.getUint16(1, false);
            const newH = data.getUint16(3, false);
            rdpWidth = newW;
            rdpHeight = newH;
            canvas.width = newW;
            canvas.height = newH;
            fitCanvas();
            setStatus(`Resized to ${newW}×${newH}`, 'success');
            break;
        }

        case MSG_CURSOR_SHAPE: {
            const hotX = data.getUint16(1, false);
            const hotY = data.getUint16(3, false);
            const curW = data.getUint16(5, false);
            const curH = data.getUint16(7, false);
            const curPixelBytes = curW * curH * 4;

            const cursorCanvas = document.createElement('canvas');
            cursorCanvas.width = curW;
            cursorCanvas.height = curH;
            const curCtx = cursorCanvas.getContext('2d');

            // BGRA → RGBA swap
            const curPixels = new Uint8ClampedArray(event.data, 9, curPixelBytes);
            for (let j = 0; j < curPixelBytes; j += 4) {
                const temp = curPixels[j];
                curPixels[j] = curPixels[j + 2];
                curPixels[j + 2] = temp;
            }

            const curImgData = new ImageData(curPixels, curW, curH);
            curCtx.putImageData(curImgData, 0, 0);

            const cursorUrl = cursorCanvas.toDataURL();
            canvas.style.cursor = `url(${cursorUrl}) ${hotX} ${hotY}, auto`;
            break;
        }

        case MSG_CLIPBOARD: {
            const length = data.getUint32(1, false);
            if (event.data.byteLength >= 5 + length) {
                const textBytes = new Uint8Array(event.data, 5, length);
                const text = new TextDecoder('utf-8').decode(textBytes);
                
                // Write to local clipboard
                if (navigator.clipboard && navigator.clipboard.writeText) {
                    navigator.clipboard.writeText(text).then(() => {
                        setStatus('Clipboard synced', 'success');
                    }).catch(err => {
                        console.warn('Clipboard write failed, focus needed?', err);
                        setStatus('Clipboard received (click to allow)', 'success');
                    });
                }
            }
            break;
        }
    }
};

// === Mouse Input ===
function sendMouse(e, flags) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    const rect = canvas.getBoundingClientRect();
    const scaleX = canvas.width / rect.width;
    const scaleY = canvas.height / rect.height;
    const x = Math.max(0, Math.min(canvas.width - 1, Math.round((e.clientX - rect.left) * scaleX)));
    const y = Math.max(0, Math.min(canvas.height - 1, Math.round((e.clientY - rect.top) * scaleY)));

    const buf = new ArrayBuffer(7);
    const view = new DataView(buf);
    view.setUint8(0, MSG_MOUSE);
    view.setUint16(1, flags, false);
    view.setUint16(3, x, false);
    view.setUint16(5, y, false);
    ws.send(buf);
}

canvas.addEventListener('mousemove', e => sendMouse(e, PTR_FLAGS_MOVE));

canvas.addEventListener('mousedown', e => {
    e.preventDefault();
    const btn = [PTR_FLAGS_BUTTON1, PTR_FLAGS_BUTTON3, PTR_FLAGS_BUTTON2][e.button] || 0;
    if (btn) sendMouse(e, btn | PTR_FLAGS_DOWN);
});

canvas.addEventListener('mouseup', e => {
    e.preventDefault();
    const btn = [PTR_FLAGS_BUTTON1, PTR_FLAGS_BUTTON3, PTR_FLAGS_BUTTON2][e.button] || 0;
    if (btn) sendMouse(e, btn); // no DOWN flag = release
});

canvas.addEventListener('wheel', e => {
    e.preventDefault();
    let flags = PTR_FLAGS_WHEEL;
    let delta = Math.round(-e.deltaY / 4);
    if (delta < 0) {
        flags |= PTR_FLAGS_WHEEL_NEGATIVE;
        delta = -delta;
    }
    delta = Math.min(delta, 0xFF);
    flags |= (delta & 0xFF);
    sendMouse(e, flags);
}, { passive: false });

canvas.addEventListener('contextmenu', e => e.preventDefault());

// === Keyboard Input ===
const SCANCODE_MAP = {
    'Escape': 0x01,
    'Digit1': 0x02, 'Digit2': 0x03, 'Digit3': 0x04, 'Digit4': 0x05,
    'Digit5': 0x06, 'Digit6': 0x07, 'Digit7': 0x08, 'Digit8': 0x09,
    'Digit9': 0x0A, 'Digit0': 0x0B,
    'Minus': 0x0C, 'Equal': 0x0D, 'Backspace': 0x0E, 'Tab': 0x0F,
    'KeyQ': 0x10, 'KeyW': 0x11, 'KeyE': 0x12, 'KeyR': 0x13,
    'KeyT': 0x14, 'KeyY': 0x15, 'KeyU': 0x16, 'KeyI': 0x17,
    'KeyO': 0x18, 'KeyP': 0x19,
    'BracketLeft': 0x1A, 'BracketRight': 0x1B, 'Enter': 0x1C,
    'ControlLeft': 0x1D,
    'KeyA': 0x1E, 'KeyS': 0x1F, 'KeyD': 0x20, 'KeyF': 0x21,
    'KeyG': 0x22, 'KeyH': 0x23, 'KeyJ': 0x24, 'KeyK': 0x25,
    'KeyL': 0x26,
    'Semicolon': 0x27, 'Quote': 0x28, 'Backquote': 0x29,
    'ShiftLeft': 0x2A, 'Backslash': 0x2B,
    'KeyZ': 0x2C, 'KeyX': 0x2D, 'KeyC': 0x2E, 'KeyV': 0x2F,
    'KeyB': 0x30, 'KeyN': 0x31, 'KeyM': 0x32,
    'Comma': 0x33, 'Period': 0x34, 'Slash': 0x35,
    'ShiftRight': 0x36, 'NumpadMultiply': 0x37,
    'AltLeft': 0x38, 'Space': 0x39, 'CapsLock': 0x3A,
    'F1': 0x3B, 'F2': 0x3C, 'F3': 0x3D, 'F4': 0x3E, 'F5': 0x3F,
    'F6': 0x40, 'F7': 0x41, 'F8': 0x42, 'F9': 0x43, 'F10': 0x44,
    'NumLock': 0x45, 'ScrollLock': 0x46,
    'Numpad7': 0x47, 'Numpad8': 0x48, 'Numpad9': 0x49, 'NumpadSubtract': 0x4A,
    'Numpad4': 0x4B, 'Numpad5': 0x4C, 'Numpad6': 0x4D, 'NumpadAdd': 0x4E,
    'Numpad1': 0x4F, 'Numpad2': 0x50, 'Numpad3': 0x51,
    'Numpad0': 0x52, 'NumpadDecimal': 0x53,
    'F11': 0x57, 'F12': 0x58,
    'NumpadEnter': 0x1C,   // extended
    'ControlRight': 0x1D,  // extended
    'NumpadDivide': 0x35,  // extended
    'AltRight': 0x38,      // extended
    'Home': 0x47,          // extended
    'ArrowUp': 0x48,       // extended
    'PageUp': 0x49,        // extended
    'ArrowLeft': 0x4B,     // extended
    'ArrowRight': 0x4D,    // extended
    'End': 0x4F,           // extended
    'ArrowDown': 0x50,     // extended
    'PageDown': 0x51,      // extended
    'Insert': 0x52,        // extended
    'Delete': 0x53,        // extended
    'MetaLeft': 0x5B,      // extended
    'MetaRight': 0x5C,     // extended
};

const EXTENDED_KEYS = new Set([
    'NumpadEnter', 'ControlRight', 'NumpadDivide', 'AltRight',
    'Home', 'ArrowUp', 'PageUp', 'ArrowLeft', 'ArrowRight',
    'End', 'ArrowDown', 'PageDown', 'Insert', 'Delete',
    'MetaLeft', 'MetaRight',
]);

function sendKey(e, down) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    const scancode = SCANCODE_MAP[e.code];
    if (scancode === undefined) return;

    e.preventDefault();

    let flags = down ? KBD_FLAGS_DOWN : KBD_FLAGS_RELEASE;
    if (EXTENDED_KEYS.has(e.code)) flags |= KBD_FLAGS_EXTENDED;

    const buf = new ArrayBuffer(5);
    const view = new DataView(buf);
    view.setUint8(0, MSG_KEYBOARD);
    view.setUint16(1, flags, false);
    view.setUint16(3, scancode, false);
    ws.send(buf);
}

document.addEventListener('keydown', e => sendKey(e, true));
document.addEventListener('keyup', e => sendKey(e, false));

// Prevent default browser shortcuts that interfere with RDP
window.addEventListener('beforeunload', () => {
    if (ws && ws.readyState === WebSocket.OPEN) ws.close();
});

// === Clipboard Event Listener ===
function sendClipboardText(text) {
    if (!ws || ws.readyState !== WebSocket.OPEN || !text) return;
    const textBytes = new TextEncoder().encode(text);
    const buf = new ArrayBuffer(5 + textBytes.length);
    const view = new DataView(buf);
    view.setUint8(0, MSG_CLIPBOARD);
    view.setUint32(1, textBytes.length, false);
    new Uint8Array(buf).set(textBytes, 5);
    ws.send(buf);
}

document.addEventListener('paste', e => {
    // Attempt to get text from paste event
    const text = (e.clipboardData || window.clipboardData).getData('text');
    if (text) {
        sendClipboardText(text);
        setStatus('Clipboard sent', 'success');
    }
});

// Since copy is intercepted by RDP sometimes, we can also poll or listen to copy events 
// but typically the remote server handles remote copy. If the user copies something 
// natively while focused on the canvas, we can intercept it:
document.addEventListener('copy', e => {
    // Allow default copy if text is selected, but usually there's no selection in canvas
    if (!window.getSelection().toString()) {
        // We could request clipboard from server, but our RDP integration already pushes it!
    }
});
