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

uint32_t abi_version(void) { return 1; }
uint32_t abi_output_ptr(void) { return (uint32_t)(uintptr_t)output; }
uint32_t abi_output_len(void) { return output_length; }
uint32_t abi_status(void) { return status; }
uint32_t abi_alloc(uint32_t size) { return (uint32_t)(uintptr_t)malloc(size); }
void abi_free(uint32_t pointer) { free((void *)(uintptr_t)pointer); }

static void clear_output(void) {
    if (output) {
        if (output_kind == 1) js_free(context, output);
        else if (output_kind == 2) JS_FreeCString(context, (const char *)output);
        else free(output);
    }
    output = NULL; output_length = 0; output_kind = 0; status = 0;
}
static void literal_error(const char *text) {
    status = 2; output_length = strlen(text); output = malloc(output_length);
    if (output) memcpy(output, text, output_length); else output_length = 0;
}
static void exception(void) {
    JSValue value = JS_GetException(context);
    size_t length = 0;
    const char *text = JS_ToCStringLen(context, &length, value);
    /* String coercion is inside Wasm execution and therefore interruptible. */
    status = 1;
    if (text) {
        output_length = length > 65536 ? 65536 : length;
        output = malloc(output_length);
        if (output) memcpy(output, text, output_length); else output_length = 0;
        JS_FreeCString(context, text);
    }
    JS_FreeValue(context, value);
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
uint32_t abi_compile(uint32_t pointer, uint32_t length) {
    clear_output();
    JSValue value = JS_Eval(context, (const char *)(uintptr_t)pointer, length,
        "rule.js", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY | JS_EVAL_FLAG_STRICT);
    if (JS_IsException(value)) { exception(); return status; }
    size_t size = 0;
    output = JS_WriteObject(context, &size, value, JS_WRITE_OBJ_BYTECODE);
    JS_FreeValue(context, value);
    if (!output) { exception(); return status; }
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
    if (JS_IsException(bridge)) { exception(); return status; }
    JSValue rule = evaluate(rule_ptr, rule_len);
    if (JS_IsException(rule)) { JS_FreeValue(context, bridge); exception(); return status; }
    if (!JS_IsFunction(context, bridge) || !JS_IsFunction(context, rule)) {
        JS_FreeValue(context, bridge); JS_FreeValue(context, rule);
        literal_error("JSONTRANS_CONVERSION: source is not a function"); return status;
    }
    JSValue args[7] = { rule,
        JS_NewStringLen(context, (const char *)(uintptr_t)input_ptr, input_len),
        JS_NewStringLen(context, (const char *)(uintptr_t)bindings_ptr, bindings_len),
        JS_NewUint32(context, depth), JS_NewUint32(context, items),
        JS_NewUint32(context, bytes), JS_NewUint32(context, work) };
    JSValue value = JS_UNDEFINED;
    for (int i = 0; i < 7; i++) if (JS_IsException(args[i])) goto failed;
    value = JS_Call(context, bridge, JS_UNDEFINED, 7, args);
    if (JS_IsException(value)) goto failed;
    size_t length = 0;
    const char *text = JS_ToCStringLen(context, &length, value);
    if (!text) goto failed;
    if (length > (uint64_t)bytes + 32) {
        JS_FreeCString(context, text);
        literal_error("JSONTRANS_LIMIT: ABI output bytes");
    } else {
        output = (uint8_t *)text; output_length = length; output_kind = 2;
    }
    goto done;
failed:
    exception();
done:
    for (int i = 0; i < 7; i++) JS_FreeValue(context, args[i]);
    JS_FreeValue(context, bridge); JS_FreeValue(context, value);
    return status;
}
