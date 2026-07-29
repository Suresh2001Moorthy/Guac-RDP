//go:build freerdp
// +build freerdp

#ifndef C_HELPERS_H
#define C_HELPERS_H

#include <stdlib.h>

#ifdef __cplusplus
extern "C" {
#endif

// Opaque context for the native FreeRDP helper. Implementation will wrap
// libfreerdp structures. The context owns any allocated frame buffers and
// synchronization primitives.
void* rdpf_client_new();
int rdpf_client_connect(void* ctx, const char* host, const char* user, const char* pass);
void rdpf_client_disconnect(void* ctx);
void rdpf_client_free(void* ctx);

// Returns a malloc'd RGBA buffer for the latest frame. Caller must free using
// rdpf_client_free_frame(). On error or if no frame available returns NULL.
unsigned char* rdpf_client_get_frame(void* ctx, int* out_width, int* out_height, int* out_stride);
void rdpf_client_free_frame(unsigned char* buf);

// Input injection APIs. Return 0 on success, non-zero on error.
int rdpf_inject_mouse(void* ctx, int x, int y, int mask);
int rdpf_inject_key(void* ctx, int scancode, int pressed);

#ifdef __cplusplus
}
#endif

#endif // C_HELPERS_H
