;; A well-behaved guest: scan copies the input it was given to the output.
(module
  (import "orbiter" "read_input" (func $read_input (param i32 i32) (result i32)))
  (import "orbiter" "write_output" (func $write_output (param i32 i32)))
  (memory (export "memory") 1)
  (func (export "scan")
    (local $n i32)
    (local.set $n (call $read_input (i32.const 1024) (i32.const 4096)))
    (call $write_output (i32.const 1024) (local.get $n))))
