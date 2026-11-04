// Owned bridge. Intrinsics are captured before evaluating user source.
(function () {
  "use strict";
  const parse = JSON.parse, quote = JSON.stringify;
  const keys = Reflect.ownKeys, descriptor = Object.getOwnPropertyDescriptor;
  const prototype = Object.getPrototypeOf, array = Array.isArray;
  const objectPrototype = Object.prototype, arrayPrototype = Array.prototype;
  const own = Object.prototype.hasOwnProperty, apply = Reflect.apply;
  const code = String.prototype.charCodeAt, finite = Number.isFinite;
  const ErrorType = Error, create = Object.create, StringType = String, slice = String.prototype.slice;
  // Reserve conversion traversal storage and failure/fallback tuples
  // before evaluating user source. None of these allocations depend on free
  // heap remaining after untrusted execution.
  const active=create(null);
  const conversionFailure=[3,"JSON result conversion failed (diagnostic unavailable)"];
  const limitFailure=[2,"JSON conversion limit exceeded (diagnostic unavailable)"];
  const evaluateFallback=[4,""], outputFallback=[5,""];
  return function (rule, inputText, bindingsText, depthLimit, itemLimit, byteLimit, workLimit, ownedMode, phaseRecord) {
    let stage = "evaluate", failure;
    function raise(kind, message) {
      if (!ownedMode) throw new ErrorType((kind === "limit" ? "JSONTRANS_LIMIT: " : "JSONTRANS_CONVERSION: ") + message);
      if (kind === "limit") { limitFailure[1]=message; failure=limitFailure; }
      else { conversionFailure[1]=message; failure=conversionFailure; }
      return false;
    }
    function success(text) { return ownedMode ? [0, text] : text; }
    function diagnostic(value) {
      const r = create(null); r.version=2; r.kind="exception"; r.stage=stage;
      r.name=""; r.message="JavaScript exception (diagnostic unavailable)"; r.stack=""; r.truncated=false;
      function bounded(text, maximum) {
        text=StringType(text); if(text.length>maximum){r.truncated=true;return apply(slice,text,[0,maximum]);} return text;
      }
      try {
        const object = value !== null && (typeof value === "object" || typeof value === "function");
        const n=object ? value.name : undefined, m=object ? value.message : undefined, stack=object ? value.stack : undefined;
        const name=n===undefined ? "" : bounded(n,256);
        const message=bounded(m===undefined ? value : m,4096);
        const trace=stack===undefined ? "" : bounded(stack,4096);
        r.name=name;r.message=message;r.stack=trace;
      } catch (_) { return stage === "output" ? outputFallback : evaluateFallback; }
      return [1, quote(r)];
    }
    try {
    const input = parse(inputText), bindings = parse(bindingsText);
    if (typeof rule !== "function") { raise("conversion", "source is not a function"); return failure; }
    // Initialize traversal closures before user code can exhaust the heap.
    // Block-scoped function declarations below create QuickJS closures when
    // their declarations execute; placing them after rule() loses provenance
    // before visit can return a reserved conversion/limit outcome.
    let output = "", bytes = 0, steps = 0;
    let activeCount = 0;
    function fail(message) { return raise("conversion", message); }
    function tick() { return ++steps <= workLimit || raise("limit", "conversion work"); }
    function write(text) {
      for (let i = 0; i < text.length; i++) {
        if (!tick()) return false;
        const c = apply(code, text, [i]);
        if (c < 128) bytes++;
        else if (c < 2048) bytes += 2;
        else if (c >= 0xd800 && c <= 0xdbff && i+1 < text.length && apply(code,text,[i+1]) >= 0xdc00 && apply(code,text,[i+1]) <= 0xdfff) { bytes += 4; i++; }
        else bytes += 3;
        if (bytes > byteLimit) return raise("limit", "output bytes");
      }
      output += text;
      return true;
    }
    function visit(value, depth) {
      if (!tick()) return false;
      if (depth >= depthLimit) return raise("limit", "output depth");
      if (value === null) return write("null");
      const type = typeof value;
      if (type === "string" || type === "boolean") return write(quote(value));
      if (type === "number") { if (!finite(value)) return fail("nonfinite number"); return write(quote(value)); }
      if (type !== "object") return fail("non-JSON value");
      for (let i=0;i<activeCount;i++) if(active[i] === value) return fail("cycle");
      const isArray = array(value), p = prototype(value);
      if (p !== null && p !== (isArray ? arrayPrototype : objectPrototype)) return fail("non-JSON object");
      active[activeCount++] = value;
      const names = keys(value);
      if (names.length > itemLimit + (isArray ? 1 : 0)) return raise("limit", "collection");
      if (isArray) {
        const length = descriptor(value,"length").value;
        if (names.length !== length + 1) return fail("extra array properties");
        if (length > itemLimit) return raise("limit", "collection");
        if (!write("[")) return false;
        for (let i=0;i<length;i++) {
          const d = descriptor(value, ""+i);
          if (!d || !apply(own,d,["value"])) return fail("array hole or accessor");
          if (i && !write(",")) return false;
          if (!visit(d.value,depth+1)) return false;
        }
        if (!write("]")) return false;
      } else {
        if (!write("{")) return false;
        let count=0;
        for(let i=0;i<names.length;i++) {
          if (!tick()) return false;
          const name = names[i];
          if(typeof name !== "string") return fail("symbol key");
          const d = descriptor(value,name);
          if(!d.enumerable) continue;
          if(!apply(own,d,["value"])) return fail("accessor");
          if (count++ && !write(",")) return false;
          if (!write(quote(name)) || !write(":") || !visit(d.value,depth+1)) return false;
        }
        if (!write("}")) return false;
      }
      delete active[--activeCount];
      return true;
    }
    const result = rule(input, bindings);
    if (ownedMode) phaseRecord[0] = 1;
    stage = "output";
    if (result === undefined) return success('{"defined":false}');
    // Trusted failures return reserved tuples. Throwing even a private token
    // asks QuickJS to allocate a backtrace, which can replace it with OOM.
    if (!visit(result,0)) return failure;
    return success('{"defined":true,"value":' + output + '}');
    } catch (value) {
      if(!ownedMode) throw value;
      try { return diagnostic(value); }
      catch (_) { return stage === "output" ? outputFallback : evaluateFallback; }
    }
  };
})()
