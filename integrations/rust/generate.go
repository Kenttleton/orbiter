package rust_integration

//go:generate sh -c "cargo build --manifest-path Cargo.toml --target wasm32-unknown-unknown --release && cp target/wasm32-unknown-unknown/release/rust.wasm ."
