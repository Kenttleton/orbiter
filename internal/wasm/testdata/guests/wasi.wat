;; Imports WASI's fd_write: a guest that wants a file system or a console.
(module
  (import "wasi_snapshot_preview1" "fd_write" (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)
  (func (export "scan") (drop (call $fd_write (i32.const 1) (i32.const 0) (i32.const 0) (i32.const 0)))))
