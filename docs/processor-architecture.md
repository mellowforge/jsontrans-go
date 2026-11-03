# Processor architecture atlas

Open the editable [FigJam atlas](https://www.figma.com/board/lTXgmGOWS3Hmh6IQzzmMmB).
It contains five labeled sections:

1. [Shared API](https://www.figma.com/board/lTXgmGOWS3Hmh6IQzzmMmB?node-id=8-500): explicit registry, compilation, input APIs and result shape.
2. [JSONPath](https://www.figma.com/board/lTXgmGOWS3Hmh6IQzzmMmB?node-id=8-502): parser, selectors, filters, RFC functions, exact numbers and match lists.
3. [JSONata](https://www.figma.com/board/lTXgmGOWS3Hmh6IQzzmMmB?node-id=8-504): AST, contexts, sequences, closures, standard library, prepared plans and raw values.
4. [Experimental JavaScript](https://www.figma.com/board/lTXgmGOWS3Hmh6IQzzmMmB?node-id=8-506): owned QuickJS/Wazero, fresh guests, capability limits and this contract PR.
5. [Prepared execution and errors](https://www.figma.com/board/lTXgmGOWS3Hmh6IQzzmMmB?node-id=8-508): reusable documents, native/prepared/fallback dispatch, aggregate limits and error propagation.

Blue/violet identify native engines; amber identifies preview JS/contract
work. QJS wrapper and Goja are assessment alternatives, not dependencies of the
owned guest. Tengo is not implemented. Runtime storage is fresh per JS evaluation;
compiled programs/documents are immutable and reusable.

The diagrams explain data/control flow, not mandatory sequential passes through
every language feature. A query need not use regex, closures or every selector.
The [compatibility policy](compatibility.md) and [JS contract](javascript-execution-contract.md)
remain the detailed behavior references.

The extracted implementation lives in the separate `javascript` module. Its
owned runtime/guest has one canonical copy; assessment alternatives delegate
to its public API. The diagram's contract-PR labels refer to the prior checkpoint.
