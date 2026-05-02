param(
    [string]$Duration = "10s",
    [int]$Workers = 0,
    [int]$Rows = 1000,
    [string]$Modes = "read,insert,select-eq,select-like,related-select,mixed"
)

$ErrorActionPreference = "Stop"

if ($Workers -le 0) {
    $Workers = [Environment]::ProcessorCount
}

$env:TSU_SPECIAL_TESTS = "1"
$env:TSU_REL_REPORT_DURATION = $Duration
$env:TSU_REL_REPORT_WORKERS = "$Workers"
$env:TSU_REL_REPORT_ROWS = "$Rows"
$env:TSU_REL_REPORT_MODES = $Modes
$env:TSU_REL_SETUP_PROGRESS = "0"

go test ./tests -run TestSpecialRelationalReport -count=1 -v
