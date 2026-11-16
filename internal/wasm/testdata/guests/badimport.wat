;; Imports a host function that does not exist, as a guest reaching for more power than it was given.
(module
  (import "orbiter" "shell_exec" (func $shell_exec (param i32 i32) (result i32)))
  (memory (export "memory") 1)
  (func (export "scan") (drop (call $shell_exec (i32.const 0) (i32.const 0)))))
