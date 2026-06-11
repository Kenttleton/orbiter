# Orbiter shell integration — fish
# Source this in ~/.config/fish/config.fish:
#   ::ORBITER:: init | source
function orbiter
    set _orbiter_out (::ORBITER:: $argv)
    set _orbiter_exit $status
    if test $_orbiter_exit -ne 0
        echo $_orbiter_out >&2
        return $_orbiter_exit
    end
    eval $_orbiter_out
end
