# Orbiter shell integration — bash
# Source this in ~/.bashrc or ~/.bash_profile:
#   eval "$(::ORBITER:: init shell)"

function orbiter() {
    local _out _exit _session="" _ran_find=0
    _out="$(::ORBITER:: "$@")"
    _exit=$?
    if [ $_exit -ne 0 ]; then
        echo "$_out" >&2
        return $_exit
    fi
    while IFS= read -r _line; do
        [[ -z "$_line" ]] && continue
        local _op="${_line%% *}"
        local _rest="${_line#* }"
        case "$_op" in
            DIR)     cd "$_rest" ;;
            SET)     export "${_rest%%=*}=${_rest#*=}" ;;
            UNSET)   unset "$_rest" ;;
            SESSION) _session="$_rest" ;;
            FIND_BINARY|FIND_DIR|FIND_ENV|FIND_FILE)
                local _txn="${_rest%% *}" _args="${_rest#* }" _key _val=""
                _key="${_args%% *}"
                [[ "$_args" == *" "* ]] && _val="${_args#* }"
                if [[ -n "$_val" ]]; then "$_op" "$_txn" "$_key" "$_val"; else "$_op" "$_txn" "$_key"; fi
                _ran_find=1
                ;;
            *)       printf '%s\n' "$_line" ;;   # pass through ordinary output
        esac
    done <<< "$_out"
    if [[ $_ran_find -eq 1 && -n "$_session" ]]; then
        orbiter uplink --session "$_session"
    fi
}

# JSON-quote one string (escapes backslash and double-quote).
function _orbiter_json_str() {
    local s="$1"
    s="${s//\\/\\\\}"
    s="${s//\"/\\\"}"
    printf '"%s"' "$s"
}

function _orbiter_find_reply() { # <txn> <json>
    printf '%s\n' "$2" | orbiter relay --txn="$1"
}

function FIND_BINARY() {
    local txn="$1" name="$2" dir p out="" seen=$'\n'
    local -a dirs
    IFS=: read -ra dirs <<< "$PATH"
    for dir in "${dirs[@]}"; do
        [ -z "$dir" ] && continue          # empty entry means cwd; never a match
        p="$dir/$name"
        [ -f "$p" ] && [ -x "$p" ] || continue   # regular executable file, not a directory
        case "$seen" in *$'\n'"$p"$'\n'*) continue ;; esac
        seen+="$p"$'\n'
        out+="${out:+,}$(_orbiter_json_str "$p")"
    done
    _orbiter_find_reply "$txn" "{\"status\":\"ok\",\"matches\":[$out]}"
}
export -f FIND_BINARY

function FIND_FILE() {
    local txn="$1" pattern="$2" location="${3:-$ORBITER_CWD}" out="" f
    location="${location/#\~/$HOME}"
    if [ ! -d "$location" ]; then
        _orbiter_find_reply "$txn" "{\"status\":\"not_found\",\"message\":$(_orbiter_json_str "location does not exist: $location")}"
        return
    fi
    while IFS= read -r f; do
        [ -z "$f" ] && continue
        out+="${out:+,}$(_orbiter_json_str "$f")"
    done < <(find "$location" -type f -name "$pattern" 2>/dev/null)
    _orbiter_find_reply "$txn" "{\"status\":\"ok\",\"matches\":[$out]}"
}
export -f FIND_FILE

function FIND_DIR() {
    local txn="$1" path="${2/#\~/$HOME}"
    if [ -d "$path" ]; then
        _orbiter_find_reply "$txn" "{\"status\":\"ok\",\"path\":$(_orbiter_json_str "$path"),\"exists\":true}"
    else
        _orbiter_find_reply "$txn" '{"status":"ok","path":"","exists":false}'
    fi
}
export -f FIND_DIR

function FIND_ENV() {
    local txn="$1" name="$2" value
    value="$(printenv "$name" 2>/dev/null)"
    _orbiter_find_reply "$txn" "{\"status\":\"ok\",\"name\":$(_orbiter_json_str "$name"),\"value\":$(_orbiter_json_str "$value")}"
}
export -f FIND_ENV

function _orbiter_hook() {
    local _prev=$?
    [[ -n "$ORBITER_CWD" && ("$PWD" == "$ORBITER_CWD" || "$PWD" == "$ORBITER_CWD/"*) ]] && return $_prev
    local _out _exit
    _out="$(::ORBITER:: hook --cwd "$PWD" --current "${ORBITER_PLANET:-}")"
    _exit=$?
    if [[ $_exit -ne 0 ]]; then echo "$_out" >&2; return $_prev; fi
    [[ -z "$_out" ]] && return $_prev
    local _new_exports=()
    while IFS= read -r _line; do
        [[ -z "$_line" ]] && continue
        local _op="${_line%% *}"
        local _rest="${_line#* }"
        case "$_op" in
            DEPART)
                for _k in $ORBITER_EXPORTS; do unset "$_k"; done
                unset ORBITER_PLANET ORBITER_EXPORTS ORBITER_CWD
                ;;
            SET)
                local _key="${_rest%%=*}"
                export "${_key}=${_rest#*=}"
                [[ "$_key" != ORBITER_* ]] && _new_exports+=("$_key")
                ;;
        esac
    done <<< "$_out"
    export ORBITER_CWD="$PWD"
    [[ ${#_new_exports[@]} -gt 0 ]] && export ORBITER_EXPORTS="${_new_exports[*]}"
    return $_prev
}

if [[ "$PROMPT_COMMAND" != *"_orbiter_hook"* ]]; then
    PROMPT_COMMAND="_orbiter_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
fi
