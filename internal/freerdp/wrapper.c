#include "wrapper.h"
#include "callbacks.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <freerdp/client.h>
#include <freerdp/client/cliprdr.h>

#ifdef _WIN32
#include <windows.h>
#include <wtsapi32.h>
#endif

extern void hook_gdi_callbacks(freerdp* instance);

typedef struct RDPContext {
    freerdp* instance;
    uintptr_t sessionID;
    void* cliprdr;
#ifdef _WIN32
    CRITICAL_SECTION lock;
#endif
} RDPContext;

#define MAX_SESSIONS 16384
static RDPContext* g_instances[MAX_SESSIONS];

uintptr_t rdp_find_session_id(freerdp* instance) {
    for (uintptr_t i = 1; i < MAX_SESSIONS; i++) {
        if (g_instances[i] && g_instances[i]->instance == instance)
            return i;
    }
    return 0;
}

static void register_session(uintptr_t id, RDPContext* ctx) {
    if (id < MAX_SESSIONS) g_instances[id] = ctx;
}

static void unregister_session(uintptr_t id) {
    if (id < MAX_SESSIONS) g_instances[id] = NULL;
}

void rdp_set_cliprdr(freerdp* instance, void* cliprdr) {
    for (uintptr_t i = 1; i < MAX_SESSIONS; i++) {
        if (g_instances[i] && g_instances[i]->instance == instance) {
            g_instances[i]->cliprdr = cliprdr;
            break;
        }
    }
}

static void log_bool_setting(rdpSettings* settings, FreeRDP_Settings_Keys_Bool key,
                             const char* name) {
    fprintf(stderr, "[RDP][caps] %-32s %s\n", name,
            freerdp_settings_get_bool(settings, key) ? "on" : "off");
}

static void configure_rendering_quality(rdpSettings* settings) {
    /*
     * This client presents FreeRDP's GDI primary framebuffer. RDPGFX needs a
     * graphics-pipeline surface bridge; advertising it in this build can
     * negotiate successfully while leaving the GDI primary black.
     */
    freerdp_settings_set_bool(settings, FreeRDP_SupportGraphicsPipeline, FALSE);
    freerdp_settings_set_bool(settings, FreeRDP_GfxH264, FALSE);
    freerdp_settings_set_bool(settings, FreeRDP_GfxAVC444, FALSE);
    freerdp_settings_set_bool(settings, FreeRDP_GfxAVC444v2, FALSE);
    freerdp_settings_set_bool(settings, FreeRDP_GfxProgressive, FALSE);
    freerdp_settings_set_bool(settings, FreeRDP_GfxProgressiveV2, FALSE);
    freerdp_settings_set_bool(settings, FreeRDP_GfxThinClient, FALSE);
    freerdp_settings_set_bool(settings, FreeRDP_GfxSmallCache, FALSE);
    freerdp_settings_set_bool(settings, FreeRDP_GfxSendQoeAck, TRUE);

    freerdp_settings_set_bool(settings, FreeRDP_RemoteFxCodec, FALSE);
    freerdp_settings_set_bool(settings, FreeRDP_NSCodec, TRUE);
    freerdp_settings_set_bool(settings, FreeRDP_NSCodecAllowSubsampling, FALSE);
    freerdp_settings_set_bool(settings, FreeRDP_NSCodecAllowDynamicColorFidelity, FALSE);
    freerdp_settings_set_uint32(settings, FreeRDP_NSCodecColorLossLevel, 0);

    freerdp_settings_set_bool(settings, FreeRDP_FrameMarkerCommandEnabled, TRUE);
    freerdp_settings_set_bool(settings, FreeRDP_FastPathOutput, TRUE);
    freerdp_settings_set_bool(settings, FreeRDP_SupportMonitorLayoutPdu, TRUE);
}

static void log_rendering_quality_settings(rdpSettings* settings) {
    fprintf(stderr, "[RDP][caps] Rendering capability advertisement\n");
    log_bool_setting(settings, FreeRDP_SupportGraphicsPipeline, "GraphicsPipeline");
    log_bool_setting(settings, FreeRDP_GfxH264, "RDPGFX H264/AVC420");
    log_bool_setting(settings, FreeRDP_GfxAVC444, "RDPGFX AVC444");
    log_bool_setting(settings, FreeRDP_GfxAVC444v2, "RDPGFX AVC444v2");
    log_bool_setting(settings, FreeRDP_GfxProgressive, "RDPGFX Progressive");
    log_bool_setting(settings, FreeRDP_GfxProgressiveV2, "RDPGFX ProgressiveV2");
    log_bool_setting(settings, FreeRDP_RemoteFxCodec, "RemoteFX legacy codec");
    log_bool_setting(settings, FreeRDP_NSCodec, "NSCodec legacy codec");
    log_bool_setting(settings, FreeRDP_NSCodecAllowSubsampling, "NSCodec subsampling");
    log_bool_setting(settings, FreeRDP_NSCodecAllowDynamicColorFidelity, "NSCodec dynamic fidelity");
    fprintf(stderr, "[RDP][caps] %-32s %u\n", "NSCodec color loss level",
            freerdp_settings_get_uint32(settings, FreeRDP_NSCodecColorLossLevel));
}

RDPContext* rdp_new(uintptr_t sessionID) {
    RDPContext* ctx = (RDPContext*)calloc(1, sizeof(RDPContext));
    if (!ctx) {
        fprintf(stderr, "[ERROR] calloc(RDPContext) failed\n");
        return NULL;
    }
    
#ifdef _WIN32
    InitializeCriticalSection(&ctx->lock);
#endif

    RDP_CLIENT_ENTRY_POINTS ep = {0};
    ep.Size = sizeof(RDP_CLIENT_ENTRY_POINTS);
    ep.Version = RDP_CLIENT_INTERFACE_VERSION;
    ep.ContextSize = sizeof(rdpContext);

    rdpContext* context = freerdp_client_context_new(&ep);
    if (!context) {
        fprintf(stderr, "[ERROR] freerdp_client_context_new() returned NULL\n");
#ifdef _WIN32
        DeleteCriticalSection(&ctx->lock);
#endif
        free(ctx);
        return NULL;
    }
    
    freerdp* instance = context->instance;
    ctx->instance = instance;
    ctx->sessionID = sessionID;

    instance->PreConnect = cb_PreConnect;
    instance->PostConnect = cb_PostConnect;
    instance->PostDisconnect = cb_PostDisconnect;
    instance->Authenticate = cb_Authenticate;
    instance->VerifyCertificateEx = cb_VerifyCertificateEx;

    register_session(sessionID, ctx);
    return ctx;
}

int rdp_connect(RDPContext* ctx, const char* address, const char* username,
                const char* password, int width, int height, int pixel_format) {
    rdpSettings* settings = ctx->instance->context->settings;

    /* Connection parameters */
    freerdp_settings_set_string(settings, FreeRDP_ServerHostname, address);
    freerdp_settings_set_string(settings, FreeRDP_Username, username);
    freerdp_settings_set_string(settings, FreeRDP_Password, password);
    
    /* Desktop geometry */
    freerdp_settings_set_uint32(settings, FreeRDP_DesktopWidth, width);
    freerdp_settings_set_uint32(settings, FreeRDP_DesktopHeight, height);
    freerdp_settings_set_uint32(settings, FreeRDP_ColorDepth, 32);
    
    /* Rendering engine */
    freerdp_settings_set_bool(settings, FreeRDP_SoftwareGdi, TRUE);
    
    /* 
     * Graphics Pipeline (RDPGFX) — disabled for now due to missing DVC hooks/H264 decoder
     * causing a black screen on this build.
     */
    /* Rendering quality and codec negotiation. */
    configure_rendering_quality(settings);

    /* Visual quality — tell the server to enable font smoothing and Aero */
    freerdp_settings_set_bool(settings, FreeRDP_AllowFontSmoothing, TRUE);
    freerdp_settings_set_bool(settings, FreeRDP_AllowDesktopComposition, TRUE);
    
    /* Certificate handling */
    freerdp_settings_set_bool(settings, FreeRDP_IgnoreCertificate, TRUE);

    /* Clipboard */
    freerdp_settings_set_bool(settings, FreeRDP_RedirectClipboard, TRUE);

    fprintf(stderr, "[RDP] Connecting to %s as %s (%dx%d)\n", address, username, width, height);
    log_rendering_quality_settings(settings);

    BOOL result = freerdp_connect(ctx->instance);
    
    if (!result) {
        UINT32 lastError = freerdp_get_last_error(ctx->instance->context);
        const char* errStr = freerdp_get_last_error_string(lastError);
        fprintf(stderr, "[ERROR] freerdp_connect failed: 0x%08X %s\n",
                lastError, errStr ? errStr : "Unknown");
    } else {
        hook_gdi_callbacks(ctx->instance);
        fprintf(stderr, "[RDP] Connected successfully\n");
    }

    return result;
}

void rdp_disconnect(RDPContext* ctx) {
    if (ctx && ctx->instance) {
        freerdp_disconnect(ctx->instance);
    }
}

void rdp_free(RDPContext* ctx) {
    if (ctx) {
        unregister_session(ctx->sessionID);
        if (ctx->instance && ctx->instance->context) {
            freerdp_client_context_free(ctx->instance->context);
        }
#ifdef _WIN32
        DeleteCriticalSection(&ctx->lock);
#endif
        free(ctx);
    }
}

int rdp_get_handles(RDPContext* ctx, void** handles, int maxHandles) {
    DWORD count = 0;
    HANDLE events[64];
    
    count = freerdp_get_event_handles(ctx->instance->context, events, 64);
    
    if (count > (DWORD)maxHandles) {
        count = maxHandles;
    }
    for (DWORD i = 0; i < count; i++) {
        handles[i] = (void*)events[i];
    }
    return (int)count;
}

int rdp_check_handles(RDPContext* ctx) {
#ifdef _WIN32
    EnterCriticalSection(&ctx->lock);
#endif
    BOOL result = freerdp_check_event_handles(ctx->instance->context);
#ifdef _WIN32
    LeaveCriticalSection(&ctx->lock);
#endif

    if (!result) {
        UINT32 lastError = freerdp_get_last_error(ctx->instance->context);
        if (lastError != 0) {
            const char* errStr = freerdp_get_last_error_string(lastError);
            fprintf(stderr, "[ERROR] check_event_handles failed: 0x%08X %s\n",
                    lastError, errStr ? errStr : "Unknown");
        }
    }
    return result ? 1 : 0;
}

int rdp_should_disconnect(RDPContext* ctx) {
    return freerdp_shall_disconnect_context(ctx->instance->context);
}

void rdp_send_mouse(RDPContext* ctx, uint16_t flags, uint16_t x, uint16_t y) {
#ifdef _WIN32
    EnterCriticalSection(&ctx->lock);
#endif
    freerdp_input_send_mouse_event(ctx->instance->context->input, flags, x, y);
#ifdef _WIN32
    LeaveCriticalSection(&ctx->lock);
#endif
}

void rdp_send_key(RDPContext* ctx, uint16_t flags, uint16_t code) {
#ifdef _WIN32
    EnterCriticalSection(&ctx->lock);
#endif
    freerdp_input_send_keyboard_event(ctx->instance->context->input, flags, code);
#ifdef _WIN32
    LeaveCriticalSection(&ctx->lock);
#endif
}

void rdp_send_unicode(RDPContext* ctx, uint16_t flags, uint16_t code) {
#ifdef _WIN32
    EnterCriticalSection(&ctx->lock);
#endif
    freerdp_input_send_unicode_keyboard_event(ctx->instance->context->input, flags, code);
#ifdef _WIN32
    LeaveCriticalSection(&ctx->lock);
#endif
}

void rdp_cliprdr_send_client_format_list(RDPContext* ctx) {
    if (!ctx || !ctx->cliprdr) return;
    CliprdrClientContext* cliprdr = (CliprdrClientContext*)ctx->cliprdr;
    
    CLIPRDR_FORMAT_LIST formatList = {0};
    formatList.common.msgType = CB_FORMAT_LIST;
    
    CLIPRDR_FORMAT formats[1];
    formats[0].formatId = CF_UNICODETEXT;
    formats[0].formatName = NULL;
    
    formatList.formats = formats;
    formatList.numFormats = 1;

#ifdef _WIN32
    EnterCriticalSection(&ctx->lock);
#endif
    cliprdr->ClientFormatList(cliprdr, &formatList);
#ifdef _WIN32
    LeaveCriticalSection(&ctx->lock);
#endif
}

void rdp_cliprdr_send_client_format_data_request(RDPContext* ctx) {
    if (!ctx || !ctx->cliprdr) return;
    CliprdrClientContext* cliprdr = (CliprdrClientContext*)ctx->cliprdr;
    
    CLIPRDR_FORMAT_DATA_REQUEST request = {0};
    request.common.msgType = CB_FORMAT_DATA_REQUEST;
    request.requestedFormatId = CF_UNICODETEXT;

#ifdef _WIN32
    EnterCriticalSection(&ctx->lock);
#endif
    cliprdr->ClientFormatDataRequest(cliprdr, &request);
#ifdef _WIN32
    LeaveCriticalSection(&ctx->lock);
#endif
}

void rdp_cliprdr_send_client_format_data_response(RDPContext* ctx, const char* data, int size) {
    if (!ctx || !ctx->cliprdr) return;
    CliprdrClientContext* cliprdr = (CliprdrClientContext*)ctx->cliprdr;
    
    CLIPRDR_FORMAT_DATA_RESPONSE response = {0};
    response.common.msgType = CB_FORMAT_DATA_RESPONSE;
    
    if (data && size > 0) {
        response.common.msgFlags = CB_RESPONSE_OK;
        response.requestedFormatData = (const BYTE*)data;
        response.common.dataLen = size;
    } else {
        response.common.msgFlags = CB_RESPONSE_FAIL;
        response.requestedFormatData = NULL;
        response.common.dataLen = 0;
    }

#ifdef _WIN32
    EnterCriticalSection(&ctx->lock);
#endif
    cliprdr->ClientFormatDataResponse(cliprdr, &response);
#ifdef _WIN32
    LeaveCriticalSection(&ctx->lock);
#endif
}
