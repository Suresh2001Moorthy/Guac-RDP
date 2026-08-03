#include "callbacks.h"
#include "wrapper.h"
#include <stdio.h>
#include <stdlib.h>

extern uintptr_t rdp_find_session_id(freerdp* instance);

#include <freerdp/client/cliprdr.h>
#include <freerdp/event.h>

static pEndPaint original_EndPaint = NULL;
static pDesktopResize original_DesktopResize = NULL;
static pBitmapUpdate original_BitmapUpdate = NULL;

UINT cb_cliprdr_server_format_list(CliprdrClientContext* context, const CLIPRDR_FORMAT_LIST* formatList) {
    BOOL hasUnicode = FALSE;
    for (UINT32 i = 0; i < formatList->numFormats; i++) {
        if (formatList->formats[i].formatId == CF_UNICODETEXT) {
            hasUnicode = TRUE;
            break;
        }
    }
    
    rdpContext* rdpctx = (rdpContext*)context->custom;
    if (hasUnicode && rdpctx && rdpctx->instance) {
        uintptr_t sessionID = rdp_find_session_id(rdpctx->instance);
        goOnClipboardFormatList(sessionID);
    }
    
    CLIPRDR_FORMAT_LIST_RESPONSE response = {0};
    response.common.msgType = CB_FORMAT_LIST_RESPONSE;
    response.common.msgFlags = CB_RESPONSE_OK;
    context->ClientFormatListResponse(context, &response);
    
    return CHANNEL_RC_OK;
}

UINT cb_cliprdr_server_format_data_request(CliprdrClientContext* context, const CLIPRDR_FORMAT_DATA_REQUEST* formatDataRequest) {
    rdpContext* rdpctx = (rdpContext*)context->custom;
    if (formatDataRequest->requestedFormatId == CF_UNICODETEXT && rdpctx && rdpctx->instance) {
        uintptr_t sessionID = rdp_find_session_id(rdpctx->instance);
        goOnClipboardDataRequest(sessionID);
    }
    return CHANNEL_RC_OK;
}

UINT cb_cliprdr_server_format_data_response(CliprdrClientContext* context, const CLIPRDR_FORMAT_DATA_RESPONSE* formatDataResponse) {
    rdpContext* rdpctx = (rdpContext*)context->custom;
    if (formatDataResponse->common.msgFlags == CB_RESPONSE_OK && rdpctx && rdpctx->instance) {
        uintptr_t sessionID = rdp_find_session_id(rdpctx->instance);
        goOnClipboardDataResponse(sessionID, (const char*)formatDataResponse->requestedFormatData, formatDataResponse->common.dataLen);
    }
    return CHANNEL_RC_OK;
}

void cb_OnChannelConnectedEventHandler(void* context, const ChannelConnectedEventArgs* e) {
    rdpContext* ctx = (rdpContext*)context;
    if (strcmp(e->name, CLIPRDR_SVC_CHANNEL_NAME) == 0) {
        CliprdrClientContext* cliprdr = (CliprdrClientContext*)e->pInterface;
        cliprdr->custom = ctx;
        cliprdr->ServerFormatList = cb_cliprdr_server_format_list;
        cliprdr->ServerFormatDataRequest = cb_cliprdr_server_format_data_request;
        cliprdr->ServerFormatDataResponse = cb_cliprdr_server_format_data_response;
        
        rdp_set_cliprdr(ctx->instance, cliprdr);
    }
}

void hook_gdi_callbacks(freerdp* instance) {
    rdpContext* context = instance->context;
    if (context && context->update) {
        original_EndPaint = context->update->EndPaint;
        original_DesktopResize = context->update->DesktopResize;
        original_BitmapUpdate = context->update->BitmapUpdate;

        context->update->EndPaint = cb_EndPaint;
        context->update->DesktopResize = cb_DesktopResize;
        context->update->BitmapUpdate = cb_BitmapUpdate;
    }
    if (context && context->pubSub) {
        PubSub_SubscribeChannelConnected(context->pubSub, cb_OnChannelConnectedEventHandler);
    }
}

BOOL cb_PreConnect(freerdp* instance) {
    /* No custom pre-connect logic needed for a headless gateway. */
    return TRUE;
}

BOOL cb_Authenticate(freerdp* instance, char** username, char** password, char** domain) {
    /* Credentials are pre-set via settings; accept unconditionally. */
    return TRUE;
}

DWORD cb_VerifyCertificateEx(freerdp* instance, const char* host, UINT16 port,
                              const char* common_name, const char* subject,
                              const char* issuer, const char* fingerprint, DWORD flags) {
    /* Accept all certificates. Production should validate or prompt. */
    return 1;
}

BOOL cb_PostConnect(freerdp* instance) {
    BOOL gdiResult = gdi_init(instance, PIXEL_FORMAT_BGRA32);
    if (!gdiResult) {
        fprintf(stderr, "[ERROR] gdi_init() failed\n");
        return FALSE;
    }
    
    rdpContext* context = instance->context;
    UINT32 width = freerdp_settings_get_uint32(context->settings, FreeRDP_DesktopWidth);
    UINT32 height = freerdp_settings_get_uint32(context->settings, FreeRDP_DesktopHeight);

    fprintf(stderr, "[RDP] PostConnect: %ux%u, GDI initialized, output=BGRA32 stride=%u\n",
            width, height, width * 4);

    uintptr_t sessionID = rdp_find_session_id(instance);
    goOnReady(sessionID, width, height);

    return TRUE;
}

BOOL cb_BitmapUpdate(rdpContext* context, const BITMAP_UPDATE* bitmap) {
    static UINT32 count = 0;
    if (count < 8) {
        fprintf(stderr, "[RDP][updates] legacy BitmapUpdate rectangles=%u\n",
                bitmap ? bitmap->number : 0);
        count++;
    }
    return original_BitmapUpdate ? original_BitmapUpdate(context, bitmap) : TRUE;
}

void cb_PostDisconnect(freerdp* instance) {
    gdi_free(instance);
}

BOOL cb_EndPaint(rdpContext* context) {
    rdpGdi* gdi = context->gdi;

    if (!gdi || !gdi->primary_buffer) {
        return TRUE;
    }

    /* Capture the invalid region BEFORE calling the original EndPaint,
     * because the original resets the invalid region. */
    int x = 0, y = 0, w = 0, h = 0;
    BOOL has_invalid = FALSE;

    if (gdi->primary && gdi->primary->hdc && gdi->primary->hdc->hwnd) {
        HGDI_RGN invalid = gdi->primary->hdc->hwnd->invalid;
        if (!invalid->null) {
            has_invalid = TRUE;
            x = invalid->x;
            y = invalid->y;
            w = invalid->w;
            h = invalid->h;
        }
    }

    /* Chain to the original GDI EndPaint so FreeRDP updates its internal state. */
    if (original_EndPaint) {
        original_EndPaint(context);
    }

    if (!has_invalid) {
        /* RDPGFX or other updates might not set the GDI invalid region.
         * Fallback to sending the full desktop. */
        x = 0;
        y = 0;
        w = freerdp_settings_get_uint32(context->settings, FreeRDP_DesktopWidth);
        h = freerdp_settings_get_uint32(context->settings, FreeRDP_DesktopHeight);
    }

    /* Clamp the invalid region to the desktop dimensions. */
    UINT32 deskWidth = freerdp_settings_get_uint32(context->settings, FreeRDP_DesktopWidth);
    UINT32 deskHeight = freerdp_settings_get_uint32(context->settings, FreeRDP_DesktopHeight);

    if (x < 0) { w += x; x = 0; }
    if (y < 0) { h += y; y = 0; }
    if (x + w > (int)deskWidth)  w = (int)deskWidth - x;
    if (y + h > (int)deskHeight) h = (int)deskHeight - y;

    if (w <= 0 || h <= 0) {
        return TRUE;
    }

    int rowStride = (int)deskWidth * 4;
    BYTE* pixels = gdi->primary_buffer + (y * rowStride) + (x * 4);

    uintptr_t sessionID = rdp_find_session_id(context->instance);
    goOnEndPaint(sessionID, x, y, w, h, pixels, rowStride);

    return TRUE;
}

BOOL cb_DesktopResize(rdpContext* context) {
    if (original_DesktopResize) {
        original_DesktopResize(context);
    }

    uintptr_t sessionID = rdp_find_session_id(context->instance);
    UINT32 width = freerdp_settings_get_uint32(context->settings, FreeRDP_DesktopWidth);
    UINT32 height = freerdp_settings_get_uint32(context->settings, FreeRDP_DesktopHeight);

    fprintf(stderr, "[RDP] DesktopResize: %ux%u\n", width, height);
    goOnDesktopResize(sessionID, width, height);
    return TRUE;
}
