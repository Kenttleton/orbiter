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
    set -l _session ''
    set -l _ran_find 0
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
            case SESSION
                set _session $_rest
            case FIND_BINARY FIND_DIR FIND_ENV FIND_FILE
                # txn, key, optional value (the value may contain spaces)
                set -l _p (string split -m 2 ' ' -- $_rest)
                $_op $_p
                set _ran_find 1
            case '*'
                echo $_line
        end
    end
    if test $_ran_find -eq 1; and test -n "$_session"
        orbiter uplink --session $_session
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

# JSON-quote one string (escapes backslash and double-quote).
function _orbiter_json_str
    set -l s (string replace -a '\\' '\\\\' -- $argv[1] | string replace -a '"' '\\"')
    printf '"%s"' $s
end

function _orbiter_find_reply # txn json
    printf '%s\n' $argv[2] | ::ORBITER:: relay --txn=$argv[1]
end

function FIND_BINARY
    set -l txn $argv[1]
    set -l name $argv[2]
    set -l items
    set -l seen
    for dir in $PATH
        test -n "$dir"; or continue
        set -l p "$dir/$name"
        test -f "$p" -a -x "$p"; or continue
        contains -- "$p" $seen; and continue
        set seen $seen "$p"
        set items $items (_orbiter_json_str "$p")
    end
    # Quote the joined list: an empty list would otherwise drop the whole argument.
    set -l joined (string join , $items)
    _orbiter_find_reply $txn "{\"status\":\"ok\",\"matches\":[$joined]}"
end

function FIND_FILE
    set -l txn $argv[1]
    set -l pattern $argv[2]
    set -l location $ORBITER_CWD
    test (count $argv) -ge 3; and set location $argv[3]
    set location (string replace -r '^~' $HOME -- $location)
    if not test -d "$location"
        _orbiter_find_reply $txn '{"status":"not_found","message":'(_orbiter_json_str "location does not exist: $location")'}'
        return
    end
    set -l items
    for f in (find "$location" -type f -name "$pattern" 2>/dev/null)
        set items $items (_orbiter_json_str "$f")
    end
    # Quote the joined list: an empty list would otherwise drop the whole argument.
    set -l joined (string join , $items)
    _orbiter_find_reply $txn "{\"status\":\"ok\",\"matches\":[$joined]}"
end

function FIND_DIR
    set -l txn $argv[1]
    set -l path (string replace -r '^~' $HOME -- $argv[2])
    if test -d "$path"
        _orbiter_find_reply $txn '{"status":"ok","path":'(_orbiter_json_str "$path")',"exists":true}'
    else
        _orbiter_find_reply $txn '{"status":"ok","path":"","exists":false}'
    end
end

function FIND_ENV
    set -l txn $argv[1]
    set -l value (printenv $argv[2] 2>/dev/null)
    _orbiter_find_reply $txn '{"status":"ok","name":'(_orbiter_json_str $argv[2])',"value":'(_orbiter_json_str "$value")'}'
end
