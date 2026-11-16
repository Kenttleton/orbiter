;; Recurses without end: the wasm stack overflows.
(module
  (memory (export "memory") 1)
  (func $f (call $f))
  (func (export "scan") (call $f)))
