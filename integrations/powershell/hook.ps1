# Orbiter shell integration — PowerShell (pwsh 7+)
# Add to your $PROFILE:
#   Invoke-Expression (& ::ORBITER:: init shell)

function Invoke-Orbiter {
    $out = & ::ORBITER:: @args
    if ($LASTEXITCODE -ne 0) {
        Write-Error $out
        return
    }
    $session = ''
    $ranFind = $false
    foreach ($line in ($out -split "`n")) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line -split ' ', 2
        $op = $parts[0]
        $rest = if ($parts.Length -gt 1) { $parts[1] } else { '' }
        switch ($op) {
            'DIR'   { Set-Location $rest }
            'SET'   {
                $kv = $rest -split '=', 2
                Set-Item "env:$($kv[0])" $kv[1]
            }
            'UNSET' { Remove-Item "env:$rest" -ErrorAction SilentlyContinue }
            'SESSION' { $session = $rest }
            { $_ -in 'FIND_BINARY', 'FIND_DIR', 'FIND_ENV', 'FIND_FILE' } {
                # txn, key, optional value (the value may contain spaces)
                $p = $rest -split ' ', 3
                if ($p.Length -ge 3) { & $op $p[0] $p[1] $p[2] } else { & $op $p[0] $p[1] }
                $ranFind = $true
            }
            default { Write-Output $line }
        }
    }
    if ($ranFind -and $session) { Invoke-Orbiter uplink --session $session }
}
Set-Alias orbiter Invoke-Orbiter

function _OrbiterHook {
    $prevEC = $LASTEXITCODE
    $cwd = (Get-Location).Path
    $planet = $env:ORBITER_CWD
    if ($planet -and ($cwd -eq $planet -or $cwd.StartsWith("$planet/"))) {
        $global:LASTEXITCODE = $prevEC; return
    }
    $out = & ::ORBITER:: hook --cwd $cwd --current "$($env:ORBITER_PLANET)"
    if ($LASTEXITCODE -ne 0) { Write-Error $out; $global:LASTEXITCODE = $prevEC; return }
    if ([string]::IsNullOrWhiteSpace($out)) { $global:LASTEXITCODE = $prevEC; return }
    $newExports = @()
    foreach ($line in ($out -split "`n")) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line -split ' ', 2
        $op = $parts[0]
        $rest = if ($parts.Length -gt 1) { $parts[1] } else { '' }
        switch ($op) {
            'DEPART' {
                foreach ($k in ($env:ORBITER_EXPORTS -split ' ')) {
                    Remove-Item "env:$k" -ErrorAction SilentlyContinue
                }
                Remove-Item env:ORBITER_PLANET, env:ORBITER_EXPORTS, env:ORBITER_CWD -ErrorAction SilentlyContinue
            }
            'SET' {
                $kv = $rest -split '=', 2
                Set-Item "env:$($kv[0])" $kv[1]
                if (-not $kv[0].StartsWith('ORBITER_')) { $newExports += $kv[0] }
            }
        }
    }
    $env:ORBITER_CWD = $cwd
    if ($newExports.Count -gt 0) { $env:ORBITER_EXPORTS = $newExports -join ' ' }
    $global:LASTEXITCODE = $prevEC
}

$ExecutionContext.SessionState.InvokeCommand.LocationChangedAction = { _OrbiterHook }

# JSON-quote one string (escapes backslash and double-quote).
function _OrbiterJson([string]$s) { '"' + ($s -replace '\\', '\\' -replace '"', '\"') + '"' }
function _OrbiterReply([string]$Txn, [string]$Json) { $Json | & ::ORBITER:: relay "--txn=$Txn" }

function FIND_BINARY {
    param([string]$Txn, [string]$Name)
    $paths = Get-Command $Name -All -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandType -eq 'Application' } |
        ForEach-Object { $_.Source -replace '\\', '/' } | Select-Object -Unique
    $items = @($paths | ForEach-Object { _OrbiterJson $_ })
    _OrbiterReply $Txn ('{"status":"ok","matches":[' + ($items -join ',') + ']}')
}

function FIND_FILE {
    param([string]$Txn, [string]$Pattern, [string]$Location = $env:ORBITER_CWD)
    if (-not (Test-Path -LiteralPath $Location -PathType Container)) {
        _OrbiterReply $Txn ('{"status":"not_found","message":' + (_OrbiterJson "location does not exist: $Location") + '}')
        return
    }
    $items = @(Get-ChildItem -LiteralPath $Location -Filter $Pattern -File -Recurse -ErrorAction SilentlyContinue |
        ForEach-Object { _OrbiterJson ($_.FullName -replace '\\', '/') })
    _OrbiterReply $Txn ('{"status":"ok","matches":[' + ($items -join ',') + ']}')
}

function FIND_DIR {
    param([string]$Txn, [string]$Path)
    if (Test-Path -LiteralPath $Path -PathType Container) {
        _OrbiterReply $Txn ('{"status":"ok","path":' + (_OrbiterJson ($Path -replace '\\', '/')) + ',"exists":true}')
    } else {
        _OrbiterReply $Txn '{"status":"ok","path":"","exists":false}'
    }
}

function FIND_ENV {
    param([string]$Txn, [string]$Name)
    $v = [Environment]::GetEnvironmentVariable($Name); if ($null -eq $v) { $v = '' }
    _OrbiterReply $Txn ('{"status":"ok","name":' + (_OrbiterJson $Name) + ',"value":' + (_OrbiterJson $v) + '}')
}
