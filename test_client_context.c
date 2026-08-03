#include <freerdp/client.h>
#include <stdio.h>

int main() {
    RDP_CLIENT_ENTRY_POINTS ep = {0};
    ep.Size = sizeof(RDP_CLIENT_ENTRY_POINTS);
    ep.Version = RDP_CLIENT_INTERFACE_VERSION;
    ep.ContextSize = sizeof(rdpClientContext);
    
    rdpContext* context = freerdp_client_context_new(&ep);
    freerdp* instance = context->instance;
    
    printf("Context: %p, Instance: %p\n", context, instance);
    
    freerdp_client_context_free(context);
    return 0;
}
