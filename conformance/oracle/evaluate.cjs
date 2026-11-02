// Development-only JSONata 2.2.2 reference oracle, one request per input line.
const readline = require('node:readline');
const jsonata = require('jsonata');

(async () => {
  const lines = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
  for await (const line of lines) {
    try {
      const request = JSON.parse(line);
      const value = await jsonata(request.expression).evaluate(request.input, request.bindings);
      process.stdout.write(JSON.stringify({ defined: value !== undefined, value }) + '\n');
    } catch (error) {
      process.stdout.write(JSON.stringify({ code: error.code, token: error.token, message: error.message }) + '\n');
    }
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
