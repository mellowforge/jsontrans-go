// Function source strings observable through regex coercion in JSONata 2.2.2.
// Copyright IBM Corp. 2016, 2018. MIT license. See conformance/testdata/jsonata/LICENSE.
// These are string data. The Go evaluator never executes them.
package jsonata

const regexArgumentFunctionSource = `async function (...params) {
                    // invoke func
                    return await apply(arg, params, null, environment);
                }`

const regexClosureFunctionSource = `function(str, fromIndex) {
            var result;
            re.lastIndex = fromIndex || 0;
            var match = re.exec(str);
            if(match !== null) {
                result = {
                    match: match[0],
                    start: match.index,
                    end: match.index + match[0].length,
                    groups: []
                };
                if(match.length > 1) {
                    for(var i = 1; i < match.length; i++) {
                        result.groups.push(match[i]);
                    }
                }
                result.next = ` + regexContinuationFunctionSource + `;
            }

            return result;
        }`

const regexContinuationFunctionSource = `function() {
                    if(re.lastIndex >= str.length) {
                        return undefined;
                    } else {
                        var next = closure(str, re.lastIndex);
                        if(next && next.match === '') {
                            // matches zero length string; this will never progress
                            throw {
                                code: "D1004",
                                stack: (new Error()).stack,
                                position: expr.position,
                                value: expr.value.source
                            };
                        }
                        return next;
                    }
                }`
