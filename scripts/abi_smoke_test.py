#!/usr/bin/env python3
import argparse
import base64
import ctypes
import json
import socket
import tempfile

class Buffer(ctypes.Structure):
    _fields_ = [("ptr", ctypes.c_void_p), ("len", ctypes.c_size_t)]

HOSTCALL = ctypes.CFUNCTYPE(ctypes.c_int, ctypes.c_void_p, ctypes.c_char_p, ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t, ctypes.POINTER(Buffer))
HOSTFREE = ctypes.CFUNCTYPE(None, ctypes.c_void_p, ctypes.c_size_t)

class HostAPI(ctypes.Structure):
    _fields_ = [("abi_version", ctypes.c_uint32), ("host_ctx", ctypes.c_void_p), ("call", HOSTCALL), ("free_buffer", HOSTFREE)]

class PluginAPI(ctypes.Structure):
    _fields_ = [("abi_version", ctypes.c_uint32), ("call", ctypes.c_void_p), ("free_buffer", ctypes.c_void_p), ("shutdown", ctypes.c_void_p)]

def free_port():
    sock = socket.socket()
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    sock.close()
    return port

def b64(data: bytes) -> str:
    return base64.b64encode(data).decode()

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("plugin", nargs="?", default="bin/warp-egress.so")
    args = parser.parse_args()
    lib = ctypes.CDLL(args.plugin)
    allocations = {}
    auth_json = {"type": "codex", "email": "tester@example.com", "access_token": "not-a-real-token"}

    def host_result(value):
        raw = json.dumps({"ok": True, "result": value}, separators=(",", ":")).encode()
        buf = ctypes.create_string_buffer(raw)
        ptr = ctypes.addressof(buf)
        allocations[ptr] = buf
        return ptr, len(raw)

    @HOSTCALL
    def host_call(_ctx, method_ptr, request_ptr, request_len, response_ptr):
        method = ctypes.string_at(method_ptr).decode()
        raw = ctypes.string_at(request_ptr, request_len) if request_ptr and request_len else b"{}"
        request = json.loads(raw)
        if method == "host.auth.list":
            value = {"files": [{"id": "codex.json", "auth_index": "idx-1", "name": "codex.json", "type": "codex", "provider": "codex", "label": "tester@example.com", "email": "tester@example.com"}]}
        elif method == "host.auth.get":
            assert request["auth_index"] == "idx-1"
            value = {"auth_index": "idx-1", "name": "codex.json", "json": auth_json}
        elif method == "host.auth.save":
            assert request["name"] == "codex.json"
            auth_json.clear()
            auth_json.update(request["json"])
            value = {"name": "codex.json", "path": "/tmp/codex.json"}
        else:
            raw_error = json.dumps({"ok": False, "error": {"code": "unsupported", "message": method}}).encode()
            buf = ctypes.create_string_buffer(raw_error)
            ptr = ctypes.addressof(buf)
            allocations[ptr] = buf
            response_ptr.contents.ptr = ptr
            response_ptr.contents.len = len(raw_error)
            return 1
        ptr, length = host_result(value)
        response_ptr.contents.ptr = ptr
        response_ptr.contents.len = length
        return 0

    @HOSTFREE
    def host_free(ptr, _length):
        allocations.pop(int(ptr), None)

    host = HostAPI(1, None, host_call, host_free)
    plugin = PluginAPI()
    lib.cliproxy_plugin_init.argtypes = [ctypes.POINTER(HostAPI), ctypes.POINTER(PluginAPI)]
    lib.cliproxy_plugin_init.restype = ctypes.c_int
    assert lib.cliproxy_plugin_init(ctypes.byref(host), ctypes.byref(plugin)) == 0
    assert plugin.abi_version == 1

    call_type = ctypes.CFUNCTYPE(ctypes.c_int, ctypes.c_char_p, ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t, ctypes.POINTER(Buffer))
    free_type = ctypes.CFUNCTYPE(None, ctypes.c_void_p, ctypes.c_size_t)
    shutdown_type = ctypes.CFUNCTYPE(None)
    call = call_type(plugin.call)
    free = free_type(plugin.free_buffer)
    shutdown = shutdown_type(plugin.shutdown)

    def invoke(method, value):
        raw = json.dumps(value, separators=(",", ":")).encode()
        array = (ctypes.c_uint8 * len(raw)).from_buffer_copy(raw) if raw else None
        output = Buffer()
        code = call(method.encode(), array, len(raw), ctypes.byref(output))
        response = ctypes.string_at(output.ptr, output.len) if output.ptr else b""
        if output.ptr:
            free(output.ptr, output.len)
        parsed = json.loads(response)
        assert code == 0 and parsed["ok"], (code, parsed)
        return parsed["result"]

    def management(path, method="GET", body=None):
        payload = b"" if body is None else json.dumps(body, separators=(",", ":")).encode()
        result = invoke("management.handle", {"Method": method, "Path": path, "Headers": {}, "Query": {}, "Body": b64(payload)})
        decoded = base64.b64decode(result["Body"])
        return result["StatusCode"], json.loads(decoded) if decoded else None

    global_port = free_port()
    profile_start = free_port()
    with tempfile.TemporaryDirectory() as directory:
        config = f'data-dir: "{directory}"\nglobal-port: {global_port}\nprofile-port-start: {profile_start}\nprofile-port-end: {profile_start + 10}\nauto-start: false\nhealth-check-interval: 0\n'
        registration = invoke("plugin.register", {"config_yaml": b64(config.encode()), "schema_version": 1})
        assert registration["metadata"]["Name"] == "warp-egress"
        routes = invoke("management.register", {})
        assert len(routes["routes"]) == 15 and routes["resources"][0]["Path"] == "/panel"

        status_code, profile = management("/warp-egress/profiles/create", "POST", {"name": "test-exit", "mode": "external", "proxy_url": "socks5://127.0.0.1:59999", "auto_start": False})
        assert status_code == 201, profile
        profile_id = profile["id"]

        status_code, result = management("/warp-egress/auth-files/assign", "POST", {"auth_index": "idx-1", "profile_id": profile_id, "apply_now": True})
        assert status_code == 200 and result["changed"] is True, result
        assert auth_json["proxy_url"] == "socks5://127.0.0.1:59999", auth_json

        status_code, files = management("/warp-egress/auth-files")
        assert status_code == 200 and files["files"][0]["effective"]["rule_type"] == "exact"
        print("ABI + host callback smoke test passed")
    shutdown()

if __name__ == "__main__":
    main()
