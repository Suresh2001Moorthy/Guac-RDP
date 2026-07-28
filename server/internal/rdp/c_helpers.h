#ifndef C_HELPERS_H
#define C_HELPERS_H

#include <stdlib.h>

#ifdef __cplusplus
extern "C" {
#endif

// Opaque context for the stub native FreeRDP helper. Real implementation
// will wrap libfreerdp structures.
void* rdpf_client_new();
int rdpf_client_connect(void* ctx, const char* host, const char* user, const char* pass);
void rdpf_client_disconnect(void* ctx);
void rdpf_client_free(void* ctx);

#ifdef __cplusplus
}
#endif

#endif // C_HELPERS_H
