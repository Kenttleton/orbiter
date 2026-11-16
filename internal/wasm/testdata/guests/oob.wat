;; Reads far outside its own linear memory.
(module
  (memory (export "memory") 1)
  (func (export "scan")
    (drop (i32.load (i32.const 2147483632)))))
