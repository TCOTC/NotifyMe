# Cleanup NotifyMe registry entries
# Delete HKEY_CURRENT_USER\Software\Classes\AppUserModelId\NotifyMe

Write-Host "Cleaning NotifyMe registry entries..." -ForegroundColor Yellow

$regPath = "HKCU:\Software\Classes\AppUserModelId\NotifyMe"

if (Test-Path $regPath) {
    try {
        Remove-Item -Path $regPath -Recurse -Force
        Write-Host "Successfully deleted registry key: $regPath" -ForegroundColor Green
    }
    catch {
        Write-Host "Failed to delete registry key: $_" -ForegroundColor Red
        exit 1
    }
}
else {
    Write-Host "Registry key does not exist, no cleanup needed" -ForegroundColor Gray
}

Write-Host "Cleanup completed!" -ForegroundColor Green

