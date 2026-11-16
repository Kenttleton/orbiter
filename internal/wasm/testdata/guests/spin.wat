;; Never returns: a guest that burns CPU forever.
(module
  (memory (export "memory") 1)
  (func (export "scan")
    (loop $forever (br $forever))))
