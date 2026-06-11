# Orbiter shell integration — bash
# Source this in ~/.bashrc or ~/.bash_profile:
#   eval "$(::ORBITER:: init)"
function orbiter() {
    local _orbiter_out
    _orbiter_out="$(::ORBITER:: "$@")"
    local _orbiter_exit=$?
    if [ $_orbiter_exit -ne 0 ]; then
        echo "$_orbiter_out" >&2
        return $_orbiter_exit
    fi
    eval "$_orbiter_out"
}
