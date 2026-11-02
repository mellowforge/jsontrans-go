# JSONPath CTS errata

The pinned CTS commit `20ebdcce5008c8a777ee9c80ad853ecf606950b5` includes two match cases whose expected values treat `^` and `$` as anchors:

- Case 516, `functions, match, explicit caret`: `$[?match(@, '^ab.*')]`.
- Case 517, `functions, match, explicit dollar`: `$[?match(@, '.*bc$')]`.

[RFC 9535 §2.4.6](https://www.rfc-editor.org/rfc/rfc9535.html#section-2.4.6) requires the second argument to conform to I-Regexp. [RFC 9485 §3](https://www.rfc-editor.org/rfc/rfc9485.html#section-3) includes `^` and `$` as normal characters, and [§4](https://www.rfc-editor.org/rfc/rfc9485.html#section-4) specifies XSD regular expression semantics. Therefore both expressions match literal `^` or `$`, and neither matches any supplied document string. The corrected expected node list is `[]` in each case. The RFC's non-normative mapping advice in §5 does not mention escaping these characters.

The runner verifies the exact case index, name, selector and original expected result before applying these two corrections; it logs each application and still executes every fixture case. The upstream snapshot is unmodified. Independent regression tests also check matching literal characters in both positive and negative examples.
