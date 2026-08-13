package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
    void* ptr;
    size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
    uint32_t abi_version;
    void* host_ctx;
    cliproxy_host_call_fn call;
    cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
    uint32_t abi_version;
    cliproxy_plugin_call_fn call;
    cliproxy_plugin_free_fn free_buffer;
    cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stored_host;

static void store_host_api(const cliproxy_host_api* host) { stored_host = host; }
static int call_host_api(const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
    if (stored_host == NULL || stored_host->call == NULL) return 1;
    return stored_host->call(stored_host->host_ctx, method, request, request_len, response);
}
static void free_host_buffer(void* ptr, size_t len) {
    if (stored_host != NULL && stored_host->free_buffer != NULL && ptr != NULL) stored_host->free_buffer(ptr, len);
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"unsafe"
)

var globalManager = NewManager()

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
	plugin.abi_version = C.uint32_t(abiVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, err := handlePluginMethod(C.GoString(method), requestBytes)
	if err != nil {
		writeResponse(response, errorEnvelope("plugin_error", err.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, length C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
	_ = length
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() { globalManager.Shutdown() }

func handlePluginMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		// CPA reuses the same RPC for register and reconfigure and requires the
		// SAME registration envelope back. If Configure fails, record the error
		// but still return a valid registration so the host keeps the plugin in
		// the active snapshot (otherwise the UI shows "未注册").
		if err := globalManager.Configure(request); err != nil {
			globalManager.setLastError(err.Error())
		}
		return okEnvelope(pluginRegistration())
	case "management.register":
		return okEnvelope(managementRoutes())
	case "management.handle":
		response, err := globalManager.HandleManagement(request)
		if err != nil {
			return nil, err
		}
		return okEnvelope(response)
	case "usage.handle":
		var record map[string]any
		if len(request) > 0 {
			if err := json.Unmarshal(request, &record); err != nil {
				return nil, err
			}
		}
		if err := globalManager.HandleUsage(record); err != nil {
			return nil, err
		}
		return okEnvelope(map[string]any{"recorded": true})
	case "request.intercept_before":
		var record map[string]any
		if len(request) > 0 {
			if err := json.Unmarshal(request, &record); err != nil {
				return nil, err
			}
		}
		if err := globalManager.HandleRequestBefore(record); err != nil {
			return nil, err
		}
		return okEnvelope(map[string]any{})
	case "request.intercept_after":
		// CPA 会为 request_interceptor 能力成对调用 before/after。xAI 质量
		// 由 usage 与流式响应回调结算，after 只需确认生命周期已接收，避免
		// 宿主把正常请求记录成插件拦截器失败。
		return okEnvelope(map[string]any{})
	case "response.intercept_stream_chunk":
		var record map[string]any
		if len(request) > 0 {
			if err := json.Unmarshal(request, &record); err != nil {
				return nil, err
			}
		}
		if err := globalManager.HandleStreamChunk(record); err != nil {
			return nil, err
		}
		return okEnvelope(map[string]any{})
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func okEnvelope(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}

func callHost(method string, payload any) (json.RawMessage, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	var response C.cliproxy_buffer
	var requestPtr *C.uint8_t
	if len(rawPayload) > 0 {
		ptr := C.CBytes(rawPayload)
		if ptr == nil {
			return nil, fmt.Errorf("allocate host callback payload")
		}
		defer C.free(ptr)
		requestPtr = (*C.uint8_t)(ptr)
	}
	code := C.call_host_api(cMethod, requestPtr, C.size_t(len(rawPayload)), &response)
	var rawResponse []byte
	if response.ptr != nil && response.len > 0 {
		rawResponse = C.GoBytes(response.ptr, C.int(response.len))
	}
	if response.ptr != nil {
		C.free_host_buffer(response.ptr, response.len)
	}
	if len(rawResponse) == 0 {
		return nil, fmt.Errorf("host callback %s returned no response, code=%d", method, int(code))
	}
	var env envelope
	if err := json.Unmarshal(rawResponse, &env); err != nil {
		return nil, fmt.Errorf("decode host callback %s: %w", method, err)
	}
	if !env.OK {
		if env.Error != nil {
			return nil, fmt.Errorf("%s: %s", env.Error.Code, env.Error.Message)
		}
		return nil, fmt.Errorf("host callback %s failed", method)
	}
	if code != 0 {
		return nil, fmt.Errorf("host callback %s returned code=%d", method, int(code))
	}
	return env.Result, nil
}

func callHostAuthList() (hostAuthListResponse, error) {
	raw, err := callHost("host.auth.list", map[string]any{})
	if err != nil {
		return hostAuthListResponse{}, err
	}
	var response hostAuthListResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return response, err
	}
	return response, nil
}

func callHostAuthGet(authIndex string) (hostAuthGetResponse, error) {
	raw, err := callHost("host.auth.get", hostAuthGetRequest{AuthIndex: authIndex})
	if err != nil {
		return hostAuthGetResponse{}, err
	}
	var response hostAuthGetResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return response, err
	}
	return response, nil
}

func callHostAuthSave(name string, payload json.RawMessage) (hostAuthSaveResponse, error) {
	raw, err := callHost("host.auth.save", hostAuthSaveRequest{Name: name, JSON: payload})
	if err != nil {
		return hostAuthSaveResponse{}, err
	}
	var response hostAuthSaveResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return response, err
	}
	return response, nil
}
