# Orbiter shell integration — bash
# Source this in ~/.bashrc or ~/.bash_profile:
#   eval "$(::ORBITER:: init shell)"

function orbiter() {
    local _out _exit
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
            DIR)   cd "$_rest" ;;
            SET)   export "${_rest%%=*}=${_rest#*=}" ;;
            UNSET) unset "$_rest" ;;
        esac
    done <<< "$_out"
}

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
