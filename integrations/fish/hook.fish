# Orbiter shell integration — fish
# Source this in ~/.config/fish/config.fish:
#   ::ORBITER:: init shell | source

function orbiter
    set _out (::ORBITER:: $argv)
    set _exit $status
    if test $_exit -ne 0
        echo $_out >&2
        return $_exit
    end
    for _line in (string split \n -- $_out)
        test -z "$_line"; and continue
        set _op (string split -m 1 ' ' -- $_line)[1]
        set _rest (string split -m 1 ' ' -- $_line)[2]
        switch $_op
            case DIR
                cd $_rest
            case SET
                set _key (string split -m 1 = -- $_rest)[1]
                set _val (string split -m 1 = -- $_rest)[2]
                set -gx $_key $_val
            case UNSET
                set -e $_rest
        end
    end
end

function _orbiter_hook --on-variable PWD
    set _prev $status
    set _cwd (pwd)
    if test -n "$ORBITER_CWD"; and string match -q "$ORBITER_CWD*" -- $_cwd
        return $_prev
    end
    set _out (::ORBITER:: hook --cwd $_cwd --current "$ORBITER_PLANET")
    set _exit $status
    if test $_exit -ne 0; echo $_out >&2; return $_prev; end
    test -z "$_out"; and return $_prev
    set -l _new_exports
    for _line in (string split \n -- $_out)
        test -z "$_line"; and continue
        set _op (string split -m 1 ' ' -- $_line)[1]
        set _rest (string split -m 1 ' ' -- $_line)[2]
        switch $_op
            case DEPART
                for _k in (string split ' ' -- $ORBITER_EXPORTS)
                    set -e $_k
                end
                set -e ORBITER_PLANET ORBITER_EXPORTS ORBITER_CWD
            case SET
                set _key (string split -m 1 = -- $_rest)[1]
                set _val (string split -m 1 = -- $_rest)[2]
                set -gx $_key $_val
                if not string match -q 'ORBITER_*' -- $_key
                    set -a _new_exports $_key
                end
        end
    end
    set -gx ORBITER_CWD $_cwd
    if test (count $_new_exports) -gt 0
        set -gx ORBITER_EXPORTS (string join ' ' $_new_exports)
    end
    return $_prev
end
