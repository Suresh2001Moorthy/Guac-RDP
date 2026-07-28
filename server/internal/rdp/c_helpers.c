#include "c_helpers.h"
#include <stdlib.h>
#include <string.h>

// Minimal stub implementation. Real libfreerdp calls will replace these
// functions later. These stubs let us validate cgo wiring and build steps.

// Allocate an opaque context (placeholder).
void* rdpf_client_new() {
    void** p = (void**)malloc(sizeof(void*));
    if (!p) return NULL;
    *p = NULL;
    return (void*)p;
}

// Placeholder connect: return 0 to indicate success.
int rdpf_client_connect(void* ctx, const char* host, const char* user, const char* pass) {
    (void)ctx;
    (void)host;
    (void)user;
    (void)pass;
    return 0;
}

// Placeholder disconnect
void rdpf_client_disconnect(void* ctx) {
    (void)ctx;
}

// Free the opaque context
void rdpf_client_free(void* ctx) {
    if (ctx) free(ctx);
}
