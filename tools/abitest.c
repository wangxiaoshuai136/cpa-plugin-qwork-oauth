#include <windows.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <stdlib.h>

typedef struct { void* ptr; size_t len; } cliproxy_buffer;
typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);
typedef struct {
    uint32_t abi_version;
    cliproxy_plugin_call_fn call;
    cliproxy_plugin_free_fn free_buffer;
    cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

typedef int (*init_fn)(void*, void*);

static cliproxy_plugin_api api;

static int call(const char* method, const char* json, char** out, size_t* outlen) {
    cliproxy_buffer buf = {0};
    int rc = api.call((char*)method, (uint8_t*)json, strlen(json), &buf);
    if (rc != 0) {
        printf("[%s] rc=%d\n", method, rc);
        return rc;
    }
    *out = (char*)buf.ptr; *outlen = buf.len;
    printf("[%s] %.*s\n", method, (int)(buf.len > 1200 ? 1200 : buf.len), (char*)buf.ptr);
    return 0;
}

int main(int argc, char** argv) {
    const char* dll = argc > 1 ? argv[1] : "qwenwork.dll";
    HMODULE h = LoadLibraryA(dll);
    if (!h) { printf("LoadLibrary failed: %lu\n", GetLastError()); return 1; }
    init_fn init = (init_fn)GetProcAddress(h, "cliproxy_plugin_init");
    if (!init) init = (init_fn)GetProcAddress(h, "cliproxyPluginInit");
    if (!init) { printf("init export missing\n"); return 1; }

    cliproxy_plugin_api out = {0};
    int rc = init(NULL, &out);
    printf("init rc=%d abi=%u\n", rc, out.abi_version);
    if (rc != 0) return 1;
    api = out;

    char *p; size_t n;
    call("plugin.register", "{\"config_yaml\":\"YXV0aF9kaXI6IEM6XFVzZXJzXFdlblBhdWxcQXBwRGF0YVxSb2FtaW5nXFF3ZW5Xb3JrQ04=\"}", &p, &n);
    call("model.static", "{}", &p, &n);
    call("executor.identifier", "{}", &p, &n);
    call("executor.execute", "{\"model\":\"flash\",\"payload\":\"eyJtb2RlbCI6ImZsYXNoIiwibWVzc2FnZXMiOlt7InJvbGUiOiJ1c2VyIiwiY29udGVudCI6IuWPquWbnuetlG9rIn1dLCJtYXhfdG9rZW5zIjo4fQ==\"}", &p, &n);

    if (api.shutdown) api.shutdown();
    return 0;
}
