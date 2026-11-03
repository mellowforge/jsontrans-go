// Assessment-only bridge. Intrinsics are captured before evaluating user source.
(function () {
  "use strict";
  const parse = JSON.parse, quote = JSON.stringify;
  const keys = Reflect.ownKeys, descriptor = Object.getOwnPropertyDescriptor;
  const prototype = Object.getPrototypeOf, array = Array.isArray;
  const objectPrototype = Object.prototype, arrayPrototype = Array.prototype;
  const own = Object.prototype.hasOwnProperty, apply = Reflect.apply;
  const code = String.prototype.charCodeAt, finite = Number.isFinite;
  const ErrorType = Error, create = Object.create;
  return function (rule, inputText, bindingsText, depthLimit, itemLimit, byteLimit, workLimit) {
    const input = parse(inputText), bindings = parse(bindingsText);
    if (typeof rule !== "function") throw new ErrorType("JSONTRANS_CONVERSION: source is not a function");
    const result = rule(input, bindings);
    if (result === undefined) return '{"defined":false}';
    let output = "", bytes = 0, steps = 0;
    const active = create(null); let activeCount = 0;
    function fail(message) { throw new ErrorType("JSONTRANS_CONVERSION: " + message); }
    function tick() { if (++steps > workLimit) throw new ErrorType("JSONTRANS_LIMIT: conversion work"); }
    function write(text) {
      for (let i = 0; i < text.length; i++) {
        tick();
        const c = apply(code, text, [i]);
        if (c < 128) bytes++;
        else if (c < 2048) bytes += 2;
        else if (c >= 0xd800 && c <= 0xdbff && i+1 < text.length && apply(code,text,[i+1]) >= 0xdc00 && apply(code,text,[i+1]) <= 0xdfff) { bytes += 4; i++; }
        else bytes += 3;
        if (bytes > byteLimit) throw new ErrorType("JSONTRANS_LIMIT: output bytes");
      }
      output += text;
    }
    function visit(value, depth) {
      tick();
      if (depth >= depthLimit) throw new ErrorType("JSONTRANS_LIMIT: output depth");
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
      if (names.length > itemLimit + (isArray ? 1 : 0)) throw new ErrorType("JSONTRANS_LIMIT: collection");
      if (isArray) {
        const length = descriptor(value,"length").value;
        if (names.length !== length + 1) fail("extra array properties");
        if (length > itemLimit) throw new ErrorType("JSONTRANS_LIMIT: collection");
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
    return '{"defined":true,"value":' + output + '}';
  };
})()
