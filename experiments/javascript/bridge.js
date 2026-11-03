// Assessment-only bridge. Intrinsics are captured before evaluating user source.
(function () {
  "use strict";
  const parse = JSON.parse, quote = JSON.stringify;
  const keys = Reflect.ownKeys, descriptor = Object.getOwnPropertyDescriptor;
  const prototype = Object.getPrototypeOf, array = Array.isArray;
  const objectPrototype = Object.prototype, arrayPrototype = Array.prototype;
  const own = Object.prototype.hasOwnProperty, apply = Reflect.apply;
  const code = String.prototype.charCodeAt, finite = Number.isFinite;
  const ErrorType = Error, create = Object.create, StringType = String, slice = String.prototype.slice;
  const WeakMapType = WeakMap, weakSet = WeakMap.prototype.set, weakGet = WeakMap.prototype.get;
  return function (rule, inputText, bindingsText, depthLimit, itemLimit, byteLimit, workLimit, ownedMode) {
    const failures = new WeakMapType();
    let stage = "evaluate";
    function raise(kind, message) {
      if (!ownedMode) throw new ErrorType((kind === "limit" ? "JSONTRANS_LIMIT: " : "JSONTRANS_CONVERSION: ") + message);
      const value = create(null); apply(weakSet, failures, [value, {kind, message}]); throw value;
    }
    function success(text) { return ownedMode ? [0, text] : text; }
    function diagnostic(value) {
      let known;
      if ((typeof value === "object" && value !== null) || typeof value === "function") known = apply(weakGet, failures, [value]);
      const r = create(null); r.version=2; r.kind=known ? known.kind : "exception"; r.stage=stage;
      r.name=""; r.message="JavaScript exception (diagnostic unavailable)"; r.stack=""; r.truncated=false;
      function bounded(text, maximum) {
        text=StringType(text); if(text.length>maximum){r.truncated=true;return apply(slice,text,[0,maximum]);} return text;
      }
      if(known) r.message=bounded(known.message,4096);
      else { try {
        const object = value !== null && (typeof value === "object" || typeof value === "function");
        const n=object ? value.name : undefined, m=object ? value.message : undefined, stack=object ? value.stack : undefined;
        const name=n===undefined ? "" : bounded(n,256);
        const message=bounded(m===undefined ? value : m,4096);
        const trace=stack===undefined ? "" : bounded(stack,4096);
        r.name=name;r.message=message;r.stack=trace;
      } catch (_) {} }
      return [1, quote(r)];
    }
    try {
    const input = parse(inputText), bindings = parse(bindingsText);
    if (typeof rule !== "function") raise("conversion", "source is not a function");
    const result = rule(input, bindings);
    stage = "output";
    if (result === undefined) return success('{"defined":false}');
    let output = "", bytes = 0, steps = 0;
    const active = create(null); let activeCount = 0;
    function fail(message) { raise("conversion", message); }
    function tick() { if (++steps > workLimit) raise("limit", "conversion work"); }
    function write(text) {
      for (let i = 0; i < text.length; i++) {
        tick();
        const c = apply(code, text, [i]);
        if (c < 128) bytes++;
        else if (c < 2048) bytes += 2;
        else if (c >= 0xd800 && c <= 0xdbff && i+1 < text.length && apply(code,text,[i+1]) >= 0xdc00 && apply(code,text,[i+1]) <= 0xdfff) { bytes += 4; i++; }
        else bytes += 3;
        if (bytes > byteLimit) raise("limit", "output bytes");
      }
      output += text;
    }
    function visit(value, depth) {
      tick();
      if (depth >= depthLimit) raise("limit", "output depth");
      if (value === null) { write("null"); return; }
      const type = typeof value;
      if (type === "string" || type === "boolean") { write(quote(value)); return; }
      if (type === "number") { if (!finite(value)) fail("nonfinite number"); write(quote(value)); return; }
      if (type !== "object") fail("non-JSON value");
      for (let i=0;i<activeCount;i++) if(active[i] === value) fail("cycle");
      const isArray = array(value), p = prototype(value);
      if (p !== null && p !== (isArray ? arrayPrototype : objectPrototype)) fail("non-JSON object");
      active[activeCount++] = value;
      const names = keys(value);
      if (names.length > itemLimit + (isArray ? 1 : 0)) raise("limit", "collection");
      if (isArray) {
        const length = descriptor(value,"length").value;
        if (names.length !== length + 1) fail("extra array properties");
        if (length > itemLimit) raise("limit", "collection");
        write("[");
        for (let i=0;i<length;i++) {
          const d = descriptor(value, ""+i);
          if (!d || !apply(own,d,["value"])) fail("array hole or accessor");
          if(i) write(","); visit(d.value,depth+1);
        }
        write("]");
      } else {
        write("{"); let count=0;
        for(let i=0;i<names.length;i++) {
          tick(); const name = names[i];
          if(typeof name !== "string") fail("symbol key");
          const d = descriptor(value,name);
          if(!d.enumerable) continue;
          if(!apply(own,d,["value"])) fail("accessor");
          if(count++) write(","); write(quote(name)); write(":"); visit(d.value,depth+1);
        }
        write("}");
      }
      delete active[--activeCount];
    }
    visit(result,0);
    return success('{"defined":true,"value":' + output + '}');
    } catch (value) { if(!ownedMode) throw value; return diagnostic(value); }
  };
})()
