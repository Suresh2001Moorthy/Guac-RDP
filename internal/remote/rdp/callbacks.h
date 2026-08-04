#ifndef CALLBACKS_H
#define CALLBACKS_H

#include <freerdp/freerdp.h>
#include <freerdp/gdi/gdi.h>
#include <stdint.h>

// Go-exported callbacks (implemented in session.go)
extern void goOnEndPaint(uintptr_t sessionID, int x, int y, int w, int h,
                          void* pixels, int stride);
extern void goOnDesktopResize(uintptr_t sessionID, int width, int height);
extern void goOnReady(uintptr_t sessionID, int width, int height);
extern void goOnClipboardFormatList(uintptr_t sessionID);
extern void goOnClipboardDataRequest(uintptr_t sessionID);
extern void goOnClipboardDataResponse(uintptr_t sessionID, const char* data, int size);

// C trampolines (registered with FreeRDP)
BOOL cb_PreConnect(freerdp* instance);
BOOL cb_PostConnect(freerdp* instance);
void cb_PostDisconnect(freerdp* instance);
BOOL cb_EndPaint(rdpContext* context);
BOOL cb_DesktopResize(rdpContext* context);
BOOL cb_BitmapUpdate(rdpContext* context, const BITMAP_UPDATE* bitmap);
BOOL cb_Authenticate(freerdp* instance, char** username, char** password, char** domain);
DWORD cb_VerifyCertificateEx(freerdp* instance, const char* host, UINT16 port, const char* common_name, const char* subject, const char* issuer, const char* fingerprint, DWORD flags);

#endif
