#ifndef WRAPPER_H
#define WRAPPER_H

#ifdef __cplusplus
extern "C" {
#endif

// Opaque struct representing our native FreeRDP context
typedef struct RDPContext RDPContext;

// Create a new native RDP context
RDPContext* rdp_new();

// Connect to the remote server using the provided context
// Returns 1 on success, 0 on failure
int rdp_connect(
    RDPContext* ctx,
    const char* address,
    const char* username,
    const char* password
);

// Disconnect the active RDP session
void rdp_disconnect(
    RDPContext* ctx
);

// Free the native RDP context and all associated resources
void rdp_free(
    RDPContext* ctx
);

#ifdef __cplusplus
}
#endif

#endif // WRAPPER_H
