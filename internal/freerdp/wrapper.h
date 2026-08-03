#ifndef WRAPPER_H
#define WRAPPER_H

#include <stdint.h>
#include <freerdp/freerdp.h>
#include <freerdp/gdi/gdi.h>

#ifdef _WIN32
#define _INC_WTSAPI
#endif

typedef struct RDPContext RDPContext;

RDPContext* rdp_new(uintptr_t sessionID);
int rdp_connect(RDPContext* ctx, const char* address, const char* username, const char* password, int width, int height, int pixel_format);
void rdp_disconnect(RDPContext* ctx);
void rdp_free(RDPContext* ctx);
void rdp_set_cliprdr(freerdp* instance, void* cliprdr);
int rdp_get_handles(RDPContext* ctx, void** handles, int maxHandles);
int rdp_check_handles(RDPContext* ctx);
int rdp_should_disconnect(RDPContext* ctx);
void rdp_send_mouse(RDPContext* ctx, uint16_t flags, uint16_t x, uint16_t y);
void rdp_send_key(RDPContext* ctx, uint16_t flags, uint16_t code);
void rdp_send_unicode(RDPContext* ctx, uint16_t flags, uint16_t code);

/* Clipboard */
void rdp_cliprdr_send_client_format_list(RDPContext* ctx);
void rdp_cliprdr_send_client_format_data_request(RDPContext* ctx);
void rdp_cliprdr_send_client_format_data_response(RDPContext* ctx, const char* data, int size);

#endif // WRAPPER_H
