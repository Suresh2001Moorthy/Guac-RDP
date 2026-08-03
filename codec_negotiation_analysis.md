# Codec Negotiation & Cryptographic Proof

To conclusively eliminate assumptions, we must interrogate the FreeRDP core and the Windows Server RDP stack. This report details exactly where codec negotiation happens, what your client is advertising, what the server will choose, and the exact C code changes required to prove it.

---

## 1. Review of Your FreeRDP Settings

In `wrapper.c`, you currently set:
```c
freerdp_settings_set_bool(settings, FreeRDP_SoftwareGdi, TRUE);
freerdp_settings_set_bool(settings, FreeRDP_RemoteFxCodec, TRUE);
freerdp_settings_set_bool(settings, FreeRDP_NSCodec, TRUE);
freerdp_settings_set_bool(settings, FreeRDP_FastPathOutput, TRUE);
```
**What is advertised:**
1. **General Capability Set**: Advertises FastPath output.
2. **Bitmap Capability Set**: Advertises 32bpp support.
3. **Surface Commands Capability Set**: Advertises `SURFCMDS_SETCMD` and `SURFCMDS_FRAMEMARKER`.
4. **Bitmap Codecs Capability Set**: Explicitly advertises the **RemoteFX** and **NSCodec** GUIDs.

*(Note: In FreeRDP 3.x, `FreeRDP_SupportGraphicsPipeline` defaults to TRUE depending on the backend, which advertises the `rdpgfx` Dynamic Virtual Channel).*

---

## 2. Which Codec Will the Server Choose?

Windows Server enforces a strict hierarchy based on the client's advertised capabilities during the `Demand Active PDU` and `Dynamic Virtual Channel` initialization:

1. **RDPGFX (AVC444 / H.264)**: If the client advertises the `rdpgfx` DVC channel and AVC444 support, Windows 10 / Server 2016+ will force this codec. It provides perfect text (lossless 4:4:4 chroma).
2. **RDPGFX (AVC420)**: Next priority. Heavily lossy on text (4:2:0 chroma subsampling).
3. **RemoteFX (RFX)**: If `rdpgfx` is unavailable, but RemoteFX is advertised in the Bitmap Codecs Capability Set, Windows Server 2012+ defaults to RemoteFX. **RemoteFX is highly lossy and ruins text.**
4. **NSCodec**: Used mostly for legacy Mac/Linux clients. Lossy.
5. **Raw Bitmap Updates**: The absolute fallback. Lossless, but uses immense bandwidth.

Because you explicitly enabled `RemoteFxCodec`, Windows Server will almost certainly choose **RemoteFX** (or RDPGFX if it initialized implicitly).

---

## 3. FreeRDP Source Code References

Negotiation and decoding occur in these specific FreeRDP files:

* **Capability Exchange**: `libfreerdp/core/capabilities.c`
  * `rdp_recv_demand_active()` parses the server's capabilities.
  * `rdp_write_bitmap_codecs_capability_set()` writes your `wrapper.c` codec settings to the wire.
* **RemoteFX Decoding**: `libfreerdp/codec/rfx.c` -> `rfx_process_message()`.
* **NSCodec Decoding**: `libfreerdp/codec/nsc.c` -> `nsc_process_message()`.
* **RDPGFX (AVC) Decoding**: `channels/rdpgfx/client/rdpgfx_main.c` -> `rdpgfx_on_caps_advertise()`.

---

## 4. How to Cryptographically Prove the Active Codec

We do not need to guess. We can instruct the FreeRDP core to explicitly dump the negotiated capability sets and decode paths in real-time.

### Step 1: Enable FreeRDP TRACE Logging in C

Modify `wrapper.c` to enforce the highest level of logging. Add this to the top of `rdp_connect`:

```c
#include <winpr/wlog.h>

int rdp_connect(RDPContext* ctx, const char* address, const char* username, const char* password, int width, int height, int pixel_format) {
    
    // FORCE WINPR TO DUMP ALL CODEC TRACES
    wLog* log = WLog_Get("com.freerdp");
    WLog_SetLogLevel(log, WLOG_TRACE);
    WLog_SetLogLevel(WLog_Get("com.freerdp.codec"), WLOG_TRACE);
    WLog_SetLogLevel(WLog_Get("com.freerdp.channels.rdpgfx"), WLOG_TRACE);
    WLog_SetLogLevel(WLog_Get("com.freerdp.core.update"), WLOG_TRACE);
    
    // ... rest of your settings ...
```

### Step 2: Intercept the Codec Callbacks

If you want absolute programmatic proof within your C wrapper, you can hook the underlying FreeRDP update callbacks (which fire *before* `cb_EndPaint`).

In `hook_gdi_callbacks` inside `callbacks.c`, add these hooks:

```c
// Define these at the top of callbacks.c
void cb_SurfaceBits(rdpContext* context, const SURFACE_BITS_COMMAND* cmd) {
    if (cmd->codecID == 3) {
        printf("[CODEC PROOF] Server sent RemoteFX payload! (CodecID 3)\n");
    } else if (cmd->codecID == 2) {
        printf("[CODEC PROOF] Server sent NSCodec payload! (CodecID 2)\n");
    } else if (cmd->codecID == 0) {
        printf("[CODEC PROOF] Server sent Uncompressed Bitmap!\n");
    }
    // We MUST forward to the GDI engine so EndPaint still triggers
    if (context->gdi && context->gdi->SurfaceBits) {
        context->gdi->SurfaceBits(context, cmd);
    }
}

void cb_BitmapUpdate(rdpContext* context, const BITMAP_UPDATE* bitmap) {
    printf("[CODEC PROOF] Server sent legacy raw BitmapUpdate!\n");
    // Forward to GDI engine
    if (context->gdi && context->gdi->BitmapUpdate) {
        context->gdi->BitmapUpdate(context, bitmap);
    }
}

// Inside hook_gdi_callbacks(freerdp* instance) add:
context->update->SurfaceBits = cb_SurfaceBits;
context->update->BitmapUpdate = cb_BitmapUpdate;
```

### Step 3: Analyze the Logs

Run your application.
1. If the console spams `[CODEC PROOF] Server sent RemoteFX payload!`, you have mathematically proven that **RemoteFX** is active. RemoteFX causes text blurring due to chroma subsampling and wavelet quantization.
2. If the console shows `[com.freerdp.channels.rdpgfx.client] - CapsAdvertise: AVC444`, you are using the modern Graphics Pipeline. If it says `AVC420`, it is lossy.

### 5. Final Codec Validation

If the output proves RemoteFX or AVC420 is active, the blur is definitively caused by the RDP server's compression pipeline. 
If the output proves uncompressed bitmaps (`cb_BitmapUpdate`) or `AVC444` are active, the backend is 100% lossless, mathematically isolating the blur to the JavaScript/CSS canvas resizing `fitCanvas()` logic detailed in the previous analysis.
