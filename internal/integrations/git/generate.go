package git

//go:generate sh -c "cd guest && cargo build --target wasm32-unknown-unknown --release && cp target/wasm32-unknown-unknown/release/git.wasm ../git.wasm"
