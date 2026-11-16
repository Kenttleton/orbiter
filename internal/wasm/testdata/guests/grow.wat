;; Asks for memory one page at a time until the host refuses, and reports how many pages it got
;; (as a little-endian u32) so a test can see where the host draws the line.
(module
  (import "orbiter" "write_output" (func $write_output (param i32 i32)))
  (memory (export "memory") 1)
  (func (export "scan")
    (local $i i32) (local $got i32)
    (block $done
      (loop $again
        (br_if $done (i32.ge_u (local.get $i) (i32.const 3000)))
        (br_if $done (i32.eq (memory.grow (i32.const 1)) (i32.const -1)))
        (local.set $got (i32.add (local.get $got) (i32.const 1)))
        (local.set $i (i32.add (local.get $i) (i32.const 1)))
        (br $again)))
    (i32.store (i32.const 0) (local.get $got))
    (call $write_output (i32.const 0) (i32.const 4))))
