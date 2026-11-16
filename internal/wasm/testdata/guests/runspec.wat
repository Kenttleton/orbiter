;; Treats its input as a run_command spec ({"cmd":..,"args":[..]}) and runs it, returning whatever
;; the host handed back. This is the guest a hostile integration would be: the test picks the spec.
(module
  (import "orbiter" "read_input" (func $read_input (param i32 i32) (result i32)))
  (import "orbiter" "write_output" (func $write_output (param i32 i32)))
  (import "orbiter" "run_command" (func $run_command (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 2)
  (func (export "scan")
    (local $n i32) (local $m i32)
    (local.set $n (call $read_input (i32.const 1024) (i32.const 4096)))
    (local.set $m (call $run_command (i32.const 1024) (local.get $n) (i32.const 8192) (i32.const 16384)))
    (call $write_output (i32.const 8192) (local.get $m))))
