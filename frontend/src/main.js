// 等待 Wails 运行时加载
function initApp() {
    // 检查 Wails 运行时是否可用
    if (typeof window.go === 'undefined' || typeof window.go.main === 'undefined' || typeof window.go.main.App === 'undefined') {
        console.log('等待 Wails 运行时加载...');
        setTimeout(initApp, 100);
        return;
    }

    console.log('Wails 运行时已加载');
    const app = window.go.main.App;

    // 处理外部链接点击，使用默认浏览器打开
    // 监听所有链接的点击事件
    document.addEventListener('click', (event) => {
        const target = event.target.closest('a');
        if (!target || !target.href) {
            return;
        }

        // 检查是否是外部链接（http:// 或 https://）
        const href = target.getAttribute('href');
        if (href && (href.startsWith('http://') || href.startsWith('https://'))) {
            // 阻止默认行为
            event.preventDefault();
            
            // 使用默认浏览器打开链接
            if (typeof window.runtime !== 'undefined' && typeof window.runtime.BrowserOpenURL === 'function') {
                window.runtime.BrowserOpenURL(href);
            } else {
                // 如果 BrowserOpenURL 不可用，回退到使用 window.open
                console.warn('BrowserOpenURL 不可用，使用 window.open 作为回退');
                window.open(href, '_blank');
            }
        }
    });

    // 跟踪用户是否修改了输入框（用于防止自动更新覆盖用户输入）
    const userModifiedFields = new Set();

    // 标记输入框为用户已修改
    function markFieldAsModified(fieldId) {
        userModifiedFields.add(fieldId);
    }

    // 清除输入框的修改标记（保存配置后）
    function clearFieldModified(fieldId) {
        userModifiedFields.delete(fieldId);
    }

    // 倒计时相关变量
    let countdownTimer = null;
    let currentCountdown = 0;
    let pollInterval = 60;
    let isRunning = false;

    // 加载配置并更新 UI
    // forceUpdate: 是否强制更新所有字段（包括有焦点的输入框）
    async function loadConfig(forceUpdate = false) {
        try {
            const config = await app.GetConfig();
            console.log('获取到的配置对象:', config);
            console.log('配置对象类型:', typeof config);
            console.log('config.github:', config?.github);
            console.log('config.ld246:', config?.ld246);
            console.log('config.github?.token:', config?.github?.token);
            console.log('config.ld246?.token:', config?.ld246?.token);
            
            if (config) {
                const pollIntervalInput = document.getElementById('poll-interval-input');
                const logLevelSelect = document.getElementById('log-level-select');
                const githubTokenInput = document.getElementById('github-token');
                const ld246TokenInput = document.getElementById('ld246-token');

                // 只在输入框没有焦点时才更新，避免覆盖用户正在输入的内容
                if (forceUpdate || document.activeElement !== pollIntervalInput) {
                    pollIntervalInput.value = config.poll_interval || 60;
                }
                if (forceUpdate || document.activeElement !== logLevelSelect) {
                    logLevelSelect.value = config.log_level || 'debug';
                }
                
                // 对于密码输入框，需要更谨慎处理：
                // 1. 如果强制更新（初始化时），无条件更新
                // 2. 如果输入框有焦点，说明用户正在编辑，不更新
                // 3. 如果用户已经修改过该字段，不更新（除非强制更新）
                // 4. 如果输入框当前为空，则更新（可能是首次加载或用户清空了）
                if (forceUpdate) {
                    // 强制更新时，无条件更新所有字段
                    const githubToken = (config.github && config.github.token) ? config.github.token : '';
                    const ld246Token = (config.ld246 && config.ld246.token) ? config.ld246.token : '';
                    
                    console.log('填充 GitHub Token:', githubToken ? '***' : '(空)');
                    console.log('填充 ld246 Token:', ld246Token ? '***' : '(空)');
                    githubTokenInput.value = githubToken;
                    ld246TokenInput.value = ld246Token;
                } else {
                    // 非强制更新时，只在输入框没有焦点且用户未修改时才更新
                    if (document.activeElement !== githubTokenInput && !userModifiedFields.has('github-token')) {
                        if (githubTokenInput.value === '') {
                            const githubToken = (config.github && config.github.token) ? config.github.token : '';
                            githubTokenInput.value = githubToken;
                        }
                    }
                    if (document.activeElement !== ld246TokenInput && !userModifiedFields.has('ld246-token')) {
                        if (ld246TokenInput.value === '') {
                            const ld246Token = (config.ld246 && config.ld246.token) ? config.ld246.token : '';
                            ld246TokenInput.value = ld246Token;
                        }
                    }
                }
            } else {
                console.warn('配置对象为空');
            }
        } catch (error) {
            console.error('加载配置失败:', error);
            console.error('错误堆栈:', error.stack);
        }
    }

    // 更新倒计时显示
    function updateCountdownDisplay() {
        const runningStatusEl = document.getElementById('running-status');
        if (!runningStatusEl) return;
        
        if (isRunning && currentCountdown > 0) {
            runningStatusEl.textContent = currentCountdown + ' 秒';
            runningStatusEl.className = 'value status-running';
        } else if (isRunning) {
            runningStatusEl.textContent = '0 秒';
            runningStatusEl.className = 'value status-running';
        } else {
            runningStatusEl.textContent = '已停止';
            runningStatusEl.className = 'value status-stopped';
        }
    }

    // 启动倒计时
    function startCountdown() {
        // 清除旧的定时器
        if (countdownTimer) {
            clearInterval(countdownTimer);
        }
        
        // 如果未运行，不启动倒计时
        if (!isRunning) {
            updateCountdownDisplay();
            return;
        }
        
        // 重置倒计时
        currentCountdown = pollInterval;
        updateCountdownDisplay();
        
        // 每秒更新倒计时
        countdownTimer = setInterval(() => {
            if (!isRunning) {
                clearInterval(countdownTimer);
                countdownTimer = null;
                updateCountdownDisplay();
                return;
            }
            
            currentCountdown--;
            if (currentCountdown <= 0) {
                currentCountdown = pollInterval; // 重新开始倒计时
            }
            updateCountdownDisplay();
        }, 1000);
    }

    // 停止倒计时
    function stopCountdown() {
        if (countdownTimer) {
            clearInterval(countdownTimer);
            countdownTimer = null;
        }
        updateCountdownDisplay();
    }

    // 加载状态
    async function loadStatus() {
        try {
            const status = await app.GetStatus();
            console.log('获取到的状态:', status);
            if (status) {
                const pollIntervalEl = document.getElementById('poll-interval');
                
                // 更新轮询间隔
                const newPollInterval = status.poll_interval || 60;
                if (pollIntervalEl) {
                    pollIntervalEl.textContent = newPollInterval + ' 秒';
                }
                
                // 如果轮询间隔改变，重置倒计时
                if (pollInterval !== newPollInterval) {
                    pollInterval = newPollInterval;
                    if (isRunning) {
                        startCountdown();
                    }
                }
                
                // 如果运行状态改变，更新倒计时
                const newIsRunning = status.running;
                if (isRunning !== newIsRunning) {
                    isRunning = newIsRunning;
                    if (isRunning) {
                        startCountdown();
                    } else {
                        stopCountdown();
                    }
                } else if (isRunning && !countdownTimer) {
                    // 如果正在运行但没有倒计时，启动倒计时
                    startCountdown();
                }
                
                console.log('状态已更新:', {
                    running: status.running,
                    poll_interval: status.poll_interval
                });
            } else {
                console.warn('状态对象为空');
            }
        } catch (error) {
            console.error('加载状态失败:', error);
            console.error('错误堆栈:', error.stack);
        }
    }

    // 格式化时间（返回相对时间和具体时间点）
    function formatTime(timestamp) {
        if (!timestamp) return '';
        
        // 判断时间戳是秒级还是毫秒级（大于 10^12 的是毫秒级）
        let date;
        if (timestamp > 1000000000000) {
            // 毫秒级时间戳
            date = new Date(timestamp);
        } else {
            // 秒级时间戳
            date = new Date(timestamp * 1000);
        }
        
        const now = new Date();
        const diff = now - date;
        
        // 格式化具体时间点
        const dateStr = date.toLocaleString('zh-CN', {
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit',
            hour12: false
        });
        
        // 计算相对时间
        let relativeStr = '';
        if (diff < 0) {
            // 未来时间，显示具体时间
            relativeStr = dateStr;
        } else if (diff < 60000) {
            // 小于 1 分钟
            relativeStr = '刚刚';
        } else if (diff < 3600000) {
            // 小于 1 小时
            relativeStr = Math.floor(diff / 60000) + ' 分钟前';
        } else if (diff < 86400000) {
            // 小于 24 小时
            relativeStr = Math.floor(diff / 3600000) + ' 小时前';
        } else if (diff < 604800000) {
            // 小于 7 天
            relativeStr = Math.floor(diff / 86400000) + ' 天前';
        } else {
            // 超过 7 天，只显示日期
            relativeStr = date.toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' });
        }
        
        // 返回相对时间和具体时间点
        return relativeStr + ' · ' + dateStr;
    }

    // 加载通知列表
    async function loadNotifications() {
        try {
            const notifications = await app.GetRecentNotifications();
            const listEl = document.getElementById('notifications-list');
            
            if (!listEl) return;
            
            if (!notifications || notifications.length === 0) {
                listEl.innerHTML = '<div class="notification-empty">暂无通知</div>';
                return;
            }
            
            listEl.innerHTML = notifications.map(notif => {
                const timeStr = formatTime(notif.time);
                const sourceStr = notif.source === 'github' ? 'GitHub' : 'ld246';
                const title = notif.title || notif.content || '无标题';
                const link = notif.link || '#';
                
                return `
                    <div class="notification-item" data-link="${link}" data-time="${notif.time}">
                        <div class="notification-header">
                            <div class="notification-title" title="${title}">${title}</div>
                            <div class="notification-source">${sourceStr}</div>
                        </div>
                        <div class="notification-time">${timeStr}</div>
                    </div>
                `;
            }).join('');
            
            // 绑定点击事件
            listEl.querySelectorAll('.notification-item').forEach(item => {
                item.addEventListener('click', () => {
                    const link = item.getAttribute('data-link');
                    if (link && link !== '#') {
                        if (typeof window.runtime !== 'undefined' && typeof window.runtime.BrowserOpenURL === 'function') {
                            window.runtime.BrowserOpenURL(link);
                        } else {
                            window.open(link, '_blank');
                        }
                    }
                });
            });
        } catch (error) {
            console.error('加载通知列表失败:', error);
        }
    }

    // 保存配置
    async function saveConfig() {
        try {
            const config = {
                poll_interval: parseInt(document.getElementById('poll-interval-input').value) || 60,
                log_level: document.getElementById('log-level-select').value || 'debug',
                github: {
                    token: document.getElementById('github-token').value || ''
                },
                ld246: {
                    token: document.getElementById('ld246-token').value || ''
                }
            };

            await app.SaveConfig(config);
            // 保存成功后，清除修改标记
            clearFieldModified('github-token');
            clearFieldModified('ld246-token');
            alert('配置已保存');
            await loadStatus();
        } catch (error) {
            console.error('保存配置失败:', error);
            alert('保存配置失败: ' + error.message);
        }
    }


    // 绑定事件
    document.getElementById('save-btn').addEventListener('click', saveConfig);
    // 刷新按钮：刷新配置和状态（不刷新页面，避免抖动）
    const refreshBtn = document.getElementById('refresh-btn');
    refreshBtn.addEventListener('click', async () => {
        // 添加视觉反馈：按钮禁用和文本变化
        const originalText = refreshBtn.textContent;
        refreshBtn.disabled = true;
        refreshBtn.textContent = '刷新中...';
        
        try {
            // 同时刷新配置和状态
            await Promise.all([
                loadConfig(true),  // 强制更新所有配置字段
                loadStatus()       // 更新状态信息
            ]);
        } catch (error) {
            console.error('刷新失败:', error);
        } finally {
            // 恢复按钮状态
            refreshBtn.disabled = false;
            refreshBtn.textContent = originalText;
        }
    });

    // 立即检查按钮：触发检查并重置倒计时
    const checkBtn = document.getElementById('check-btn');
    checkBtn.addEventListener('click', async () => {
        // 添加视觉反馈：按钮禁用和文本变化
        const originalText = checkBtn.textContent;
        checkBtn.disabled = true;
        checkBtn.textContent = '检查中...';
        
        try {
            // 触发后端检查
            await app.TriggerCheck();
            
            // 重置倒计时
            if (isRunning) {
                startCountdown();
            }
            
            // 等待一小段时间后刷新通知列表（给后端时间处理）
            setTimeout(async () => {
                await loadNotifications();
            }, 1000);
        } catch (error) {
            console.error('触发检查失败:', error);
        } finally {
            // 恢复按钮状态
            checkBtn.disabled = false;
            checkBtn.textContent = originalText;
        }
    });

    // 监听密码输入框的输入事件，标记为用户已修改
    const githubTokenInput = document.getElementById('github-token');
    const ld246TokenInput = document.getElementById('ld246-token');
    
    githubTokenInput.addEventListener('input', () => {
        markFieldAsModified('github-token');
    });
    
    ld246TokenInput.addEventListener('input', () => {
        markFieldAsModified('ld246-token');
    });

    // 初始化（强制更新所有字段）
    loadConfig(true);
    loadStatus();
    loadNotifications();

    // 定期刷新状态和通知列表（每2秒刷新一次，确保及时显示新通知）
    setInterval(() => {
        loadStatus();
        loadNotifications();
    }, 2000);

    // 定期更新时间显示（每秒更新一次，让相对时间更准确）
    setInterval(() => {
        const listEl = document.getElementById('notifications-list');
        if (!listEl) return;
        
        const items = listEl.querySelectorAll('.notification-item');
        items.forEach(item => {
            const timeEl = item.querySelector('.notification-time');
            if (!timeEl) return;
            
            // 从 data 属性获取原始时间戳
            const timestamp = item.getAttribute('data-time');
            if (!timestamp) return;
            
            const timeStr = formatTime(parseInt(timestamp));
            timeEl.textContent = timeStr;
        });
    }, 1000);
}

// 等待 DOM 加载完成后再初始化
if (document.readyState === 'loading') {
    window.addEventListener('DOMContentLoaded', initApp);
} else {
    // DOM 已经加载完成，直接初始化
    initApp();
}
