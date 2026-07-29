//go:build freerdp
// +build freerdp

#include "c_helpers.h"
#include <stdlib.h>
#include <string.h>

#ifdef _WIN32
#include <windows.h>
#else
#include <pthread.h>
#endif

// This implementation is a cross-platform helper that manages an opaque
// FreeRDP-related context and a single-slot latest-frame buffer. The real
// libfreerdp integration will populate the latest_frame pointer inside the
// bitmap/update callback. For now the connect functions are placeholders and
// input injection functions are stubs that return success.

typedef struct rdpf_ctx {
    void* freerdp_instance; // placeholder for eventual freerdp instance
    unsigned char* latest_frame;
    int width;
    int height;
    int stride; // bytes per row
#ifdef _WIN32
    CRITICAL_SECTION lock;
#else
    pthread_mutex_t lock;
#endif
} rdpf_ctx;

void* rdpf_client_new() {
    rdpf_ctx* c = (rdpf_ctx*)malloc(sizeof(rdpf_ctx));
    if (!c) return NULL;
    c->freerdp_instance = NULL;
    c->latest_frame = NULL;
    c->width = 0;
    c->height = 0;
    c->stride = 0;
#ifdef _WIN32
    InitializeCriticalSection(&c->lock);
#else
    pthread_mutex_init(&c->lock, NULL);
#endif
    return (void*)c;
}

int rdpf_client_connect(void* ctx, const char* host, const char* user, const char* pass) {
    (void)host;
    (void)user;
    (void)pass;
    if (!ctx) return -1;
    // Placeholder: real implementation will initialize freerdp instance,
    // set callbacks, and start connection. Return 0 for success for now.
    return 0;
}

void rdpf_client_disconnect(void* ctx) {
    if (!ctx) return;
    rdpf_ctx* c = (rdpf_ctx*)ctx;
    // Placeholder: real implementation will disconnect freerdp instance.
    (void)c;
}

void rdpf_client_free(void* ctx) {
    if (!ctx) return;
    rdpf_ctx* c = (rdpf_ctx*)ctx;
#ifdef _WIN32
    EnterCriticalSection(&c->lock);
#else
    pthread_mutex_lock(&c->lock);
#endif
    if (c->latest_frame) {
        free(c->latest_frame);
        c->latest_frame = NULL;
    }
#ifdef _WIN32
    LeaveCriticalSection(&c->lock);
    DeleteCriticalSection(&c->lock);
#else
    pthread_mutex_unlock(&c->lock);
    pthread_mutex_destroy(&c->lock);
#endif
    free(c);
}

unsigned char* rdpf_client_get_frame(void* ctx, int* out_width, int* out_height, int* out_stride) {
    if (!ctx) return NULL;
    rdpf_ctx* c = (rdpf_ctx*)ctx;
#ifdef _WIN32
    EnterCriticalSection(&c->lock);
#else
    pthread_mutex_lock(&c->lock);
#endif
    if (!c->latest_frame) {
#ifdef _WIN32
        LeaveCriticalSection(&c->lock);
#else
        pthread_mutex_unlock(&c->lock);
#endif
        return NULL;
    }
    // Transfer ownership of the frame buffer to the caller. Caller must free
    // it using rdpf_client_free_frame.
    unsigned char* frame = c->latest_frame;
    c->latest_frame = NULL;
    if (out_width) *out_width = c->width;
    if (out_height) *out_height = c->height;
    if (out_stride) *out_stride = c->stride;
#ifdef _WIN32
    LeaveCriticalSection(&c->lock);
#else
    pthread_mutex_unlock(&c->lock);
#endif
    return frame;
}

void rdpf_client_free_frame(unsigned char* buf) {
    if (buf) free(buf);
}

int rdpf_inject_mouse(void* ctx, int x, int y, int mask) {
    (void)ctx; (void)x; (void)y; (void)mask;
    // Placeholder: real implementation will call FreeRDP input APIs.
    return 0;
}

int rdpf_inject_key(void* ctx, int scancode, int pressed) {
    (void)ctx; (void)scancode; (void)pressed;
    // Placeholder: real implementation will call FreeRDP input APIs.
    return 0;
}
