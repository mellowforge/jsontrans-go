/* Owned-patch lifetime check. MIT. Development-only: never linked into Wasm. */
#include "quickjs.h"
#include <assert.h>
#include <stdio.h>
#include <string.h>

static void check_context_collection(JSRuntime *rt) {
    JSMemoryUsage usage;
    JS_RunGC(rt);
    JS_ComputeMemoryUsage(rt, &usage);
    if (usage.obj_count != 0) {
        fprintf(stderr, "retained objects after context collection: %lld\n", (long long)usage.obj_count);
        assert(usage.obj_count == 0);
    }
}

static void check_partial_error_construction(void) {
    const char *message = "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz";
    int terminal = 0, complete = 0;
    for (size_t slack = 0; slack <= 4096; slack += 8) {
        JSRuntime *rt = JS_NewRuntime();
        assert(rt);
        JSContext *ctx = JS_NewContext(rt);
        assert(ctx);
        JSMemoryUsage usage;
        JS_ComputeMemoryUsage(rt, &usage);
        JS_SetMemoryLimit(rt, usage.malloc_size + slack);
        assert(JS_IsException(JS_ThrowTypeError(ctx, "%s", message)));
        JSValue error = JS_GetException(ctx);
        JS_SetMemoryLimit(rt, (size_t)-1);
        if (JS_IsUncatchableError(error)) {
            terminal++;
        } else {
            /* A successful ordinary error must have its complete message.
             * Allocation failure must propagate the terminal error instead. */
            JSValue value = JS_GetPropertyStr(ctx, error, "message");
            const char *actual = JS_ToCString(ctx, value);
            assert(actual && strcmp(actual, message) == 0);
            JS_FreeCString(ctx, actual);
            JS_FreeValue(ctx, value);
            complete++;
        }
        JS_FreeValue(ctx, error);
        JS_FreeContext(ctx);
        check_context_collection(rt);
        JS_FreeRuntime(rt);
    }
    assert(terminal > 0 && complete > 0);
}

int main(void) {
    check_partial_error_construction();
    JSRuntime *rt = JS_NewRuntime();
    assert(rt);
    JS_SetDumpFlags(rt, JS_ABORT_ON_LEAKS | JS_DUMP_LEAKS);
    const char *source = "(()=>{globalThis.keep=null;try{for(;;)keep={next:keep}}catch(e){return 42}})()";
    for (int i = 0; i < 12; i++) {
        JS_SetMemoryLimit(rt, (size_t)-1);
        JSContext *ctx = JS_NewContext(rt);
        assert(ctx);
        JS_SetMemoryLimit(rt, 4 << 20);
        JSValue value = JS_Eval(ctx, source, strlen(source), "terminal-oom.js", JS_EVAL_TYPE_GLOBAL);
        assert(JS_IsException(value));
        JSValue error = JS_GetException(ctx);
        assert(JS_IsUncatchableError(error));
        JS_FreeValue(ctx, error);
        JS_FreeContext(ctx);
        check_context_collection(rt);
    }
    JS_FreeRuntime(rt);

    /* Cover partially initialized contexts, including failures before and after
     * the reserved error is allocated, with independent fresh runtimes. */
    for (size_t limit = 32768; limit <= 524288; limit += 8192) {
        rt = JS_NewRuntime();
        assert(rt);
        JS_SetDumpFlags(rt, JS_ABORT_ON_LEAKS | JS_DUMP_LEAKS);
        JS_SetMemoryLimit(rt, limit);
        JSContext *ctx = JS_NewContext(rt);
        if (ctx) JS_FreeContext(ctx);
        JS_FreeRuntime(rt);
    }
    puts("terminal OOM, GC roots and context-failure cleanup: PASS");
    return 0;
}
