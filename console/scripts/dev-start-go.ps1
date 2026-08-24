# ============================================================================
# [DEPRECATED] This script has been consolidated into scripts/dev/
# Please update your references to:
#   powershell -ExecutionPolicy Bypass -File scripts\dev\dev-start-go.ps1
# ============================================================================
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$ProjectRoot = Split-Path -Parent (Split-Path -Parent $ScriptDir)
$TargetScript = Join-Path $ProjectRoot "scripts\dev\dev-start-go.ps1"

Write-Warning "[DEPRECATED] console\scripts\dev-start-go.ps1 has moved to scripts\dev\dev-start-go.ps1"
& powershell -ExecutionPolicy Bypass -File $TargetScript $args
