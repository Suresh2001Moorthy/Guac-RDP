#include "wrapper.h"
#include <freerdp/freerdp.h>
#include <freerdp/settings.h>
#include <stdlib.h>
#include <string.h>

// Define our opaque structure wrapping the freerdp instance
struct RDPContext {
    freerdp* instance;
};

// Required by Windows for FreeRDP to initialize networking/API properly
static int wts_initialized = 0;

RDPContext* rdp_new() {
    if (!wts_initialized) {
        WTSRegisterWtsApi();
        wts_initialized = 1;
    }

    freerdp* instance = freerdp_new();
    if (!instance) {
        return NULL;
    }

    // Allocate the base context size
    if (!freerdp_context_new(instance)) {
        freerdp_free(instance);
        return NULL;
    }

    RDPContext* ctx = (RDPContext*)malloc(sizeof(RDPContext));
    if (!ctx) {
        freerdp_context_free(instance);
        freerdp_free(instance);
        return NULL;
    }

    ctx->instance = instance;
    return ctx;
}

int rdp_connect(
    RDPContext* ctx,
    const char* address,
    const char* username,
    const char* password
) {
    if (!ctx || !ctx->instance || !ctx->instance->settings) {
        return 0;
    }

    rdpSettings* settings = ctx->instance->settings;

#if defined(FREERDP_VERSION_MAJOR) && (FREERDP_VERSION_MAJOR >= 3)
    freerdp_settings_set_string(settings, FreeRDP_ServerHostname, address);
    freerdp_settings_set_string(settings, FreeRDP_Username, username);
    freerdp_settings_set_string(settings, FreeRDP_Password, password);
    freerdp_settings_set_bool(settings, FreeRDP_IgnoreCertificate, TRUE);
#else
    // FreeRDP v2 legacy assignment fallback
    settings->ServerHostname = _strdup(address);
    settings->Username = _strdup(username);
    settings->Password = _strdup(password);
    settings->IgnoreCertificate = TRUE;
#endif

    // Milestone 1: Minimal software rendering configuration
    settings->SoftwareGdi = TRUE;
    settings->AudioPlayback = FALSE;
    settings->FastPathOutput = TRUE;

    // Connect synchronously
    BOOL success = freerdp_connect(ctx->instance);
    return success ? 1 : 0;
}

void rdp_disconnect(RDPContext* ctx) {
    if (ctx && ctx->instance) {
        freerdp_disconnect(ctx->instance);
    }
}

void rdp_free(RDPContext* ctx) {
    if (ctx) {
        if (ctx->instance) {
            freerdp_context_free(ctx->instance);
            freerdp_free(ctx->instance);
        }
        free(ctx);
    }
}
