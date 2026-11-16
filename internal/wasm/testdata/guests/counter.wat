;; Counts its own calls in linear memory and reports the count (little-endian u32). A stateless
;; integration reports 1 every time, because every call starts from a fresh instance.
(module
  (import "orbiter" "write_output" (func $write_output (param i32 i32)))
  (memory (export "memory") 1)
  (func (export "scan")
    (i32.store (i32.const 0) (i32.add (i32.load (i32.const 0)) (i32.const 1)))
    (call $write_output (i32.const 0) (i32.const 4))))
