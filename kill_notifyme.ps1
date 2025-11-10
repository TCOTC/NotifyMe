# NotifyMe 进程查找和终止脚本

Write-Host "正在查找 NotifyMe 进程..." -ForegroundColor Yellow

# 方法 1: 通过进程名查找（可能的进程名）
$processNames = @("notifyme", "NotifyMe", "notifyme.exe", "NotifyMe.exe")

# 方法 2: 通过窗口标题查找
$windowTitles = @("NotifyMe", "notifyme")

# 方法 3: 通过路径查找
$processes = Get-Process | Where-Object {
    $proc = $_
    $found = $false
    
    # 检查进程名
    foreach ($name in $processNames) {
        if ($proc.ProcessName -like "*$name*") {
            $found = $true
            break
        }
    }
    
    # 检查窗口标题
    if (-not $found) {
        foreach ($title in $windowTitles) {
            if ($proc.MainWindowTitle -like "*$title*") {
                $found = $true
                break
            }
        }
    }
    
    # 检查路径
    if (-not $found -and $proc.Path) {
        if ($proc.Path -like "*NotifyMe*" -or $proc.Path -like "*notifyme*") {
            $found = $true
        }
    }
    
    $found
}

if ($processes) {
    Write-Host "找到以下进程:" -ForegroundColor Green
    $processes | Format-Table Id, ProcessName, MainWindowTitle, Path -AutoSize
    
    $pids = $processes | ForEach-Object { $_.Id }
    Write-Host "进程 ID: $($pids -join ', ')" -ForegroundColor Cyan
    
    $confirm = Read-Host "是否要终止这些进程? (Y/N)"
    if ($confirm -eq "Y" -or $confirm -eq "y") {
        foreach ($proc in $processes) {
            try {
                Stop-Process -Id $proc.Id -Force
                Write-Host "已终止进程: $($proc.ProcessName) (PID: $($proc.Id))" -ForegroundColor Green
            } catch {
                Write-Host "终止进程失败: $($proc.ProcessName) (PID: $($proc.Id)) - $_" -ForegroundColor Red
            }
        }
    } else {
        Write-Host "已取消操作" -ForegroundColor Yellow
    }
} else {
    Write-Host "未找到 NotifyMe 进程" -ForegroundColor Red
    Write-Host ""
    Write-Host "提示: 如果程序确实在运行，可以尝试以下方法:" -ForegroundColor Yellow
    Write-Host "1. 查看系统托盘图标（任务栏右下角）" -ForegroundColor White
    Write-Host "2. 在任务管理器中查看所有进程（包括后台进程）" -ForegroundColor White
    Write-Host "3. 使用命令: Get-Process | Format-Table Id, ProcessName, Path -AutoSize" -ForegroundColor White
}

