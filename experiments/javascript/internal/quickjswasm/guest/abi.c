/* Project-owned assessment ABI. MIT; QuickJS-ng is linked under LICENSE.quickjs. */
#include "quickjs.h"
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <sys/time.h>
#include <time.h>

/* Each module owns a time snapshot, supplied before creating its JS context.
 * No clocks, environment, I/O, entropy service or callback is imported. */
static uint64_t epoch_us;
int gettimeofday(struct timeval *restrict tv, void *restrict zone) {
    (void)zone; tv->tv_sec = epoch_us / 1000000; tv->tv_usec = epoch_us % 1000000; return 0;
}
int clock_gettime(clockid_t id, struct timespec *ts) {
    (void)id; ts->tv_sec = epoch_us / 1000000; ts->tv_nsec = (epoch_us % 1000000) * 1000; return 0;
}
/* Core diagnostics cannot acquire a host output capability. */
int printf(const char *restrict format, ...) { (void)format; return 0; }
int fprintf(FILE *restrict stream, const char *restrict format, ...) { (void)stream; (void)format; return 0; }
int puts(const char *text) { (void)text; return 0; }
int fputs(const char *restrict text, FILE *restrict stream) { (void)text; (void)stream; return 0; }
_Noreturn void abort(void) { __builtin_trap(); }

/* Deny libc's latent OS operations inside the guest. These definitions resolve
 * WASI symbols at link time; the artifact has no imported host functions. */
uint16_t __wrap___wasi_clock_time_get(uint32_t id, uint64_t precision, uint64_t *out) {
    (void)id; (void)precision; *out = epoch_us * 1000; return 0;
}
uint16_t __wrap___wasi_fd_close(uint32_t fd) { (void)fd; return 76; }
uint16_t __wrap___wasi_fd_seek(uint32_t fd, int64_t offset, uint32_t whence, uint64_t *out) {
    (void)fd; (void)offset; (void)whence; (void)out; return 76;
}
uint16_t __wrap___wasi_fd_write(uint32_t fd, const void *iov, uint32_t count, uint32_t *out) {
    (void)fd; (void)iov; (void)count; if (out) *out = 0; return 76;
}
_Noreturn void __wrap___wasi_proc_exit(uint32_t code) { (void)code; __builtin_trap(); }

static JSRuntime *runtime;
static JSContext *context;
static uint8_t *output;
static uint32_t output_length, status;
static uint32_t output_kind;

uint32_t abi_version(void) { return 2; }
uint32_t abi_output_ptr(void) { return (uint32_t)(uintptr_t)output; }
uint32_t abi_output_len(void) { return output_length; }
uint32_t abi_status(void) { return status; }
uint32_t abi_alloc(uint32_t size) { return (uint32_t)(uintptr_t)malloc(size); }
void abi_free(uint32_t pointer) { free((void *)(uintptr_t)pointer); }

static void clear_output(void) {
    if (output) {
        if (output_kind == 1) js_free(context, output);
        else if (output_kind == 2) JS_FreeCString(context, (const char *)output);
        else if (output_kind != 3) free(output);
    }
    output = NULL; output_length = 0; output_kind = 0; status = 0;
}
/* Bounded ASCII JSON diagnostic encoding. CESU-8 is escaped by UTF-16 unit,
 * preserving isolated surrogates without invoking mutable JS serializers. */
static char diagnostic_buffer[65536];
static uint32_t diagnostic_length;
static int diagnostic_truncated;
static void diagnostic_put(char c) {
    if (diagnostic_length < sizeof(diagnostic_buffer)) diagnostic_buffer[diagnostic_length++] = c;
}
static void diagnostic_raw(const char *s) { while (*s) diagnostic_put(*s++); }
static void diagnostic_quote(const char *s, size_t length, uint32_t cap) {
    static const char hex[] = "0123456789abcdef";
    uint32_t start = diagnostic_length;
    diagnostic_put('"');
    for (size_t i = 0; i < length;) {
        if (diagnostic_length - start + 8 > cap) { diagnostic_truncated = 1; break; }
        uint32_t c = (uint8_t)s[i++];
        if (c >= 0xc0 && c < 0xe0 && i < length) c = ((c & 31) << 6) | (s[i++] & 63);
        else if (c >= 0xe0 && i + 1 < length) { c = ((c & 15) << 12) | ((s[i] & 63) << 6) | (s[i+1] & 63); i += 2; }
        if (c >= 32 && c < 127 && c != '"' && c != '\\') diagnostic_put((char)c);
        else {
            diagnostic_raw("\\u");
            diagnostic_put(hex[(c >> 12) & 15]); diagnostic_put(hex[(c >> 8) & 15]);
            diagnostic_put(hex[(c >> 4) & 15]); diagnostic_put(hex[c & 15]);
        }
    }
    diagnostic_put('"');
}
static void diagnostic_begin(const char *kind, const char *stage) {
    diagnostic_length = 0; diagnostic_truncated = 0;
    diagnostic_raw("{\"version\":2,\"kind\":\""); diagnostic_raw(kind);
    diagnostic_raw("\",\"stage\":\""); diagnostic_raw(stage); diagnostic_raw("\"");
}
static void diagnostic_finish(void) {
    diagnostic_raw(diagnostic_truncated ? ",\"truncated\":true}" : ",\"truncated\":false}");
    output = (uint8_t *)diagnostic_buffer; output_length = diagnostic_length; output_kind = 3; status = 1;
}
static void literal_failure(const char *kind, const char *stage, const char *text) {
    diagnostic_begin(kind, stage);
    diagnostic_raw(",\"name\":\"\",\"message\":"); diagnostic_quote(text, strlen(text), 16000);
    diagnostic_raw(",\"stack\":\"\""); diagnostic_finish();
}
static int diagnostic_field(const char *key, JSValueConst value, uint32_t cap) {
    size_t length = 0;
    const char *text = JS_ToCStringLen2(context, &length, value, true);
    if (!text) return -1;
    diagnostic_raw(key); diagnostic_quote(text, length, cap); JS_FreeCString(context, text); return 0;
}
static void exception(const char *stage) {
    JSValue value = JS_GetException(context), name = JS_UNDEFINED, message = JS_UNDEFINED, stack = JS_UNDEFINED;
    if (JS_IsObject(value)) {
        name = JS_GetPropertyStr(context, value, "name");
        if (JS_IsException(name)) goto unavailable;
        message = JS_GetPropertyStr(context, value, "message");
        if (JS_IsException(message)) goto unavailable;
        stack = JS_GetPropertyStr(context, value, "stack");
        if (JS_IsException(stack)) goto unavailable;
    }
    diagnostic_begin("exception", stage);
    if (JS_IsUndefined(name)) diagnostic_raw(",\"name\":\"\"");
    else if (diagnostic_field(",\"name\":", name, 1024)) goto unavailable;
    if (diagnostic_field(",\"message\":", JS_IsUndefined(message) ? value : message, 16000)) goto unavailable;
    if (JS_IsUndefined(stack)) diagnostic_raw(",\"stack\":\"\"");
    else if (diagnostic_field(",\"stack\":", stack, 32000)) goto unavailable;
    diagnostic_finish(); goto done;
unavailable:
    { JSValue secondary = JS_GetException(context); JS_FreeValue(context, secondary); }
    literal_failure("exception", stage, "JavaScript exception (diagnostic unavailable)");
done:
    JS_FreeValue(context, name); JS_FreeValue(context, message); JS_FreeValue(context, stack); JS_FreeValue(context, value);
}
uint32_t abi_init(uint32_t heap_limit, uint32_t reserved_stack, uint32_t now_low, uint32_t now_high) {
    if (runtime || reserved_stack) return 2;
    epoch_us = ((uint64_t)now_high << 32) | now_low;
    runtime = JS_NewRuntime();
    if (!runtime) return 2;
    JS_SetMemoryLimit(runtime, heap_limit);
    context = JS_NewContext(runtime);
    if (!context) return 2;
    return 0;
}
uint32_t abi_compile(uint32_t pointer, uint32_t length, uint32_t source_kind) {
    clear_output();
    if (source_kind > 1) { literal_failure("internal", "compile", "invalid source kind"); return status; }
    JSValue value = JS_Eval(context, (const char *)(uintptr_t)pointer, length,
        source_kind ? "bridge.js" : "rule.js", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY | JS_EVAL_FLAG_STRICT);
    if (JS_IsException(value)) { exception("compile"); return status; }
    size_t size = 0;
    output = JS_WriteObject(context, &size, value, JS_WRITE_OBJ_BYTECODE);
    JS_FreeValue(context, value);
    if (!output) { exception("compile"); return status; }
    output_length = size; output_kind = 1;
    return 0;
}
static JSValue evaluate(uint32_t pointer, uint32_t length) {
    JSValue code = JS_ReadObject(context, (const uint8_t *)(uintptr_t)pointer, length, JS_READ_OBJ_BYTECODE);
    if (JS_IsException(code)) return code;
    return JS_EvalFunction(context, code);
}
uint32_t abi_eval(uint32_t rule_ptr, uint32_t rule_len, uint32_t bridge_ptr, uint32_t bridge_len,
    uint32_t input_ptr, uint32_t input_len, uint32_t bindings_ptr, uint32_t bindings_len,
    uint32_t depth, uint32_t items, uint32_t bytes, uint32_t work) {
    clear_output();
    JSValue bridge = evaluate(bridge_ptr, bridge_len);
    if (JS_IsException(bridge)) { exception("evaluate"); return status; }
    JSValue rule = evaluate(rule_ptr, rule_len);
    if (JS_IsException(rule)) { JS_FreeValue(context, bridge); exception("evaluate"); return status; }
    if (!JS_IsFunction(context, bridge) || !JS_IsFunction(context, rule)) {
        JS_FreeValue(context, bridge); JS_FreeValue(context, rule);
        literal_failure("conversion", "evaluate", "source is not a function"); return status;
    }
    JSValue args[8] = { rule,
        JS_NewStringLen(context, (const char *)(uintptr_t)input_ptr, input_len),
        JS_NewStringLen(context, (const char *)(uintptr_t)bindings_ptr, bindings_len),
        JS_NewUint32(context, depth), JS_NewUint32(context, items),
        JS_NewUint32(context, bytes), JS_NewUint32(context, work), JS_TRUE };
    JSValue value = JS_UNDEFINED;
    for (int i = 0; i < 8; i++) if (JS_IsException(args[i])) goto failed;
    value = JS_Call(context, bridge, JS_UNDEFINED, 8, args);
    if (JS_IsException(value)) goto failed;
    if (!JS_IsArray(value)) { literal_failure("internal", "evaluate", "invalid bridge response"); goto done; }
    JSValue outcome = JS_GetPropertyUint32(context, value, 0);
    JSValue payload = JS_GetPropertyUint32(context, value, 1);
    int32_t failed_status = -1;
    if (!JS_IsNumber(outcome) || JS_ToInt32(context, &failed_status, outcome) || (failed_status < 0 || failed_status > 5) || !JS_IsString(payload)) {
        JS_FreeValue(context, outcome); JS_FreeValue(context, payload);
        literal_failure("internal", "evaluate", "invalid bridge response"); goto done;
    }
    JS_FreeValue(context, outcome);
    /* The immutable bridge owns these preallocated outcomes. Classification
     * survives even if no JS heap remains to encode or copy diagnostic text. */
    if (failed_status >= 2) {
        if (failed_status >= 4) {
            literal_failure("exception", failed_status == 5 ? "output" : "evaluate",
                "JavaScript exception (diagnostic unavailable)");
        } else {
            const char *kind = failed_status == 2 ? "limit" : "conversion";
            const char *message = JS_ToCString(context, payload);
            if (message) {
                literal_failure(kind, "output", message);
                JS_FreeCString(context, message);
            } else {
                JSValue secondary = JS_GetException(context); JS_FreeValue(context, secondary);
                literal_failure(kind, "output", "JSON result conversion failed (diagnostic unavailable)");
            }
        }
        JS_FreeValue(context, payload); goto done;
    }
    size_t length = 0;
    const char *text = JS_ToCStringLen(context, &length, payload);
    JS_FreeValue(context, payload);
    if (!text) goto failed;
    if (length > (failed_status ? 65536 : (uint64_t)bytes + 32)) {
        JS_FreeCString(context, text);
        literal_failure("limit", "output", "ABI output bytes");
    } else {
        output = (uint8_t *)text; output_length = length; output_kind = 2; status = failed_status;
    }
    goto done;
failed:
    exception("evaluate");
done:
    for (int i = 0; i < 8; i++) JS_FreeValue(context, args[i]);
    JS_FreeValue(context, bridge); JS_FreeValue(context, value);
    return status;
}
