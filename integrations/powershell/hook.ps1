# Orbiter shell integration — PowerShell (pwsh 7+)
# Add to your $PROFILE:
#   Invoke-Expression (& ::ORBITER:: init shell)

function Invoke-Orbiter {
    $out = & ::ORBITER:: @args
    if ($LASTEXITCODE -ne 0) {
        Write-Error $out
        return
    }
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
        }
    }
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
