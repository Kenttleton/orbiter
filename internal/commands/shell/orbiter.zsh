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
            SET)   local _val="${_rest#*=}"; _val="${_val//\\n/$'\n'}"; export "${_rest%%=*}=${_val}" ;;
            UNSET) unset "$_rest" ;;
        esac
    done <<< "$_out"
}
