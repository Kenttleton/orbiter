# Orbiter shell integration — zsh
# Source this in ~/.zshrc:
#   eval "$(::ORBITER:: init shell)"

function orbiter() {
    local _out _exit
    _out="$(::ORBITER:: "$@")"
    _exit=$?
    if [ $_exit -ne 0 ]; then
        print "$_out" >&2
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

function _orbiter_chpwd() {
    local _prev=$?
    [[ "$PWD" == "$ORBITER_CWD" || "$PWD" == "$ORBITER_CWD/"* ]] && return $_prev
    local _out _exit
    _out="$(::ORBITER:: hook --cwd "$PWD" --current "${ORBITER_PLANET:-}")"
    _exit=$?
    if [[ $_exit -ne 0 ]]; then print "$_out" >&2; return $_prev; fi
    [[ -z "$_out" ]] && return $_prev
    local -a _new_exports
    while IFS= read -r _line; do
        [[ -z "$_line" ]] && continue
        local _op="${_line%% *}"
        local _rest="${_line#* }"
        case "$_op" in
            DEPART)
                for _k in ${(z)ORBITER_EXPORTS}; do unset "$_k"; done
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

autoload -Uz add-zsh-hook
add-zsh-hook chpwd _orbiter_chpwd
