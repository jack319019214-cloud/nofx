import { useEffect, useState } from 'react'
import useSWR, { useSWRConfig } from 'swr'
import { api } from './lib/api'
import { EquityChart } from './components/EquityChart'
import { AITradersPage } from './components/AITradersPage'
import { LoginPage } from './components/LoginPage'
import { RegisterPage } from './components/RegisterPage'
import { ResetPasswordPage } from './components/ResetPasswordPage'
import { CompetitionPage } from './components/CompetitionPage'
import { LandingPage } from './pages/LandingPage'
import { FAQPage } from './pages/FAQPage'
import HeaderBar from './components/landing/HeaderBar'
import AILearning from './components/AILearning'
import { LanguageProvider, useLanguage } from './contexts/LanguageContext'
import { AuthProvider, useAuth } from './contexts/AuthContext'
import { t, type Language } from './i18n/translations'
import { useSystemConfig } from './hooks/useSystemConfig'
import { AlertTriangle } from 'lucide-react'
import type {
  SystemStatus,
  AccountInfo,
  Position,
  DecisionRecord,
  Statistics,
  TraderInfo,
} from './types'

type Page = 'competition' | 'traders' | 'trader'

// 获取友好的AI模型名称
function getModelDisplayName(modelId: string): string {
  switch (modelId.toLowerCase()) {
    case 'deepseek':
      return 'DeepSeek'
    case 'qwen':
      return 'Qwen'
    case 'claude':
      return 'Claude'
    default:
      return modelId.toUpperCase()
  }
}

function App() {
  const { language, setLanguage } = useLanguage()
  const { user, token, logout, isLoading } = useAuth()
  const { loading: configLoading } = useSystemConfig()
  const [route, setRoute] = useState(window.location.pathname)

  // 从URL路径读取初始页面状态（支持刷新保持页面）
  const getInitialPage = (): Page => {
    const path = window.location.pathname
    const hash = window.location.hash.slice(1) // 去掉 #

    if (path === '/traders' || hash === 'traders') return 'traders'
    if (path === '/dashboard' || hash === 'trader' || hash === 'details')
      return 'trader'
    return 'competition' // 默认为竞赛页面
  }

  const [currentPage, setCurrentPage] = useState<Page>(getInitialPage())
  const [selectedTraderId, setSelectedTraderId] = useState<string | undefined>()
  const [lastUpdate, setLastUpdate] = useState<string>('--:--:--')

  // 监听URL变化，同步页面状态
  useEffect(() => {
    const handleRouteChange = () => {
      const path = window.location.pathname
      const hash = window.location.hash.slice(1)

      if (path === '/traders' || hash === 'traders') {
        setCurrentPage('traders')
      } else if (
        path === '/dashboard' ||
        hash === 'trader' ||
        hash === 'details'
      ) {
        setCurrentPage('trader')
      } else if (
        path === '/competition' ||
        hash === 'competition' ||
        hash === ''
      ) {
        setCurrentPage('competition')
      }
      setRoute(path)
    }

    window.addEventListener('hashchange', handleRouteChange)
    window.addEventListener('popstate', handleRouteChange)
    return () => {
      window.removeEventListener('hashchange', handleRouteChange)
      window.removeEventListener('popstate', handleRouteChange)
    }
  }, [])

  // 切换页面时更新URL hash (当前通过按钮直接调用setCurrentPage，这个函数暂时保留用于未来扩展)
  // const navigateToPage = (page: Page) => {
  //   setCurrentPage(page);
  //   window.location.hash = page === 'competition' ? '' : 'trader';
  // };

  // 获取trader列表（仅在用户登录时）
  const { data: traders } = useSWR<TraderInfo[]>(
    user && token ? 'traders' : null,
    api.getTraders,
    {
      refreshInterval: 10000,
    }
  )

  // 当获取到traders后，设置默认选中第一个
  useEffect(() => {
    if (traders && traders.length > 0 && !selectedTraderId) {
      setSelectedTraderId(traders[0].trader_id)
    }
  }, [traders, selectedTraderId])

  // 如果在trader页面，获取该trader的数据
  const { data: status } = useSWR<SystemStatus>(
    currentPage === 'trader' && selectedTraderId
      ? `status-${selectedTraderId}`
      : null,
    () => api.getStatus(selectedTraderId),
    {
      refreshInterval: 15000, // 15秒刷新（配合后端15秒缓存）
      revalidateOnFocus: false, // 禁用聚焦时重新验证，减少请求
      dedupingInterval: 10000, // 10秒去重，防止短时间内重复请求
    }
  )

  const { data: account } = useSWR<AccountInfo>(
    currentPage === 'trader' && selectedTraderId
      ? `account-${selectedTraderId}`
      : null,
    () => api.getAccount(selectedTraderId),
    {
      refreshInterval: 15000, // 15秒刷新（配合后端15秒缓存）
      revalidateOnFocus: false, // 禁用聚焦时重新验证，减少请求
      dedupingInterval: 10000, // 10秒去重，防止短时间内重复请求
    }
  )

  const { data: positions } = useSWR<Position[]>(
    currentPage === 'trader' && selectedTraderId
      ? `positions-${selectedTraderId}`
      : null,
    () => api.getPositions(selectedTraderId),
    {
      refreshInterval: 5000, // 5秒刷新（实时更新持仓状态）
      revalidateOnFocus: true, // 启用聚焦时重新验证（用户切换回页面时立即刷新）
      dedupingInterval: 3000, // 3秒去重，允许更频繁的刷新
    }
  )

  const { data: decisions } = useSWR<DecisionRecord[]>(
    currentPage === 'trader' && selectedTraderId
      ? `decisions/latest-${selectedTraderId}`
      : null,
    () => api.getLatestDecisions(selectedTraderId),
    {
      refreshInterval: 30000, // 30秒刷新（决策更新频率较低）
      revalidateOnFocus: false,
      dedupingInterval: 20000,
    }
  )

  const { data: stats } = useSWR<Statistics>(
    currentPage === 'trader' && selectedTraderId
      ? `statistics-${selectedTraderId}`
      : null,
    () => api.getStatistics(selectedTraderId),
    {
      refreshInterval: 30000, // 30秒刷新（统计数据更新频率较低）
      revalidateOnFocus: false,
      dedupingInterval: 20000,
    }
  )

  useEffect(() => {
    if (account) {
      const now = new Date().toLocaleTimeString()
      setLastUpdate(now)
    }
  }, [account])

  const selectedTrader = traders?.find((t) => t.trader_id === selectedTraderId)

  // Handle routing
  useEffect(() => {
    const handlePopState = () => {
      setRoute(window.location.pathname)
    }
    window.addEventListener('popstate', handlePopState)
    return () => window.removeEventListener('popstate', handlePopState)
  }, [])

  // Set current page based on route for consistent navigation state
  useEffect(() => {
    if (route === '/competition') {
      setCurrentPage('competition')
    } else if (route === '/traders') {
      setCurrentPage('traders')
    } else if (route === '/dashboard') {
      setCurrentPage('trader')
    }
  }, [route])

  // Show loading spinner while checking auth or config
  if (isLoading || configLoading) {
    return (
      <div
        className="min-h-screen flex items-center justify-center"
        style={{ background: '#0B0E11' }}
      >
        <div className="text-center">
          <img
            src="/icons/nofx.svg"
            alt="NoFx Logo"
            className="w-16 h-16 mx-auto mb-4 animate-pulse"
          />
          <p style={{ color: '#EAECEF' }}>{t('loading', language)}</p>
        </div>
      </div>
    )
  }

  // Handle specific routes regardless of authentication
  if (route === '/login') {
    return <LoginPage />
  }
  if (route === '/register') {
    return <RegisterPage />
  }
  if (route === '/faq') {
    return <FAQPage />
  }
  if (route === '/reset-password') {
    return <ResetPasswordPage />
  }
  if (route === '/competition') {
    return (
      <div
        className="min-h-screen"
        style={{ background: '#000000', color: '#EAECEF' }}
      >
        <HeaderBar
          isLoggedIn={!!user}
          currentPage="competition"
          language={language}
          onLanguageChange={setLanguage}
          user={user}
          onLogout={logout}
          onPageChange={(page) => {
            console.log('Competition page onPageChange called with:', page)
            console.log('Current route:', route, 'Current page:', currentPage)

            if (page === 'competition') {
              console.log('Navigating to competition')
              window.history.pushState({}, '', '/competition')
              setRoute('/competition')
              setCurrentPage('competition')
            } else if (page === 'traders') {
              console.log('Navigating to traders')
              window.history.pushState({}, '', '/traders')
              setRoute('/traders')
              setCurrentPage('traders')
            } else if (page === 'trader') {
              console.log('Navigating to trader/dashboard')
              window.history.pushState({}, '', '/dashboard')
              setRoute('/dashboard')
              setCurrentPage('trader')
            } else if (page === 'faq') {
              console.log('Navigating to faq')
              window.history.pushState({}, '', '/faq')
              setRoute('/faq')
            }

            console.log(
              'After navigation - route:',
              route,
              'currentPage:',
              currentPage
            )
          }}
        />
        <main className="max-w-[1920px] mx-auto px-6 py-6 pt-24">
          <CompetitionPage />
        </main>
      </div>
    )
  }

  // Show landing page for root route
  if (route === '/' || route === '') {
    return <LandingPage />
  }

  // Show main app for authenticated users on other routes
  if (!user || !token) {
    // Default to landing page when not authenticated and no specific route
    return <LandingPage />
  }

  return (
    <div
      className="min-h-screen"
      style={{ background: '#0B0E11', color: '#EAECEF' }}
    >
      <HeaderBar
        isLoggedIn={!!user}
        currentPage={currentPage}
        language={language}
        onLanguageChange={setLanguage}
        user={user}
        onLogout={logout}
        onPageChange={(page) => {
          console.log('Main app onPageChange called with:', page)

          if (page === 'competition') {
            window.history.pushState({}, '', '/competition')
            setRoute('/competition')
            setCurrentPage('competition')
          } else if (page === 'traders') {
            window.history.pushState({}, '', '/traders')
            setRoute('/traders')
            setCurrentPage('traders')
          } else if (page === 'trader') {
            window.history.pushState({}, '', '/dashboard')
            setRoute('/dashboard')
            setCurrentPage('trader')
          } else if (page === 'faq') {
            window.history.pushState({}, '', '/faq')
            setRoute('/faq')
          }
        }}
      />

      {/* Main Content */}
      <main className="max-w-[1920px] mx-auto px-6 py-6 pt-24">
        {currentPage === 'competition' ? (
          <CompetitionPage />
        ) : currentPage === 'traders' ? (
          <AITradersPage
            onTraderSelect={(traderId) => {
              setSelectedTraderId(traderId)
              window.history.pushState({}, '', '/dashboard')
              setRoute('/dashboard')
              setCurrentPage('trader')
            }}
          />
        ) : (
          <TraderDetailsPage
            selectedTrader={selectedTrader}
            status={status}
            account={account}
            positions={positions}
            decisions={decisions}
            stats={stats}
            lastUpdate={lastUpdate}
            language={language}
            traders={traders}
            selectedTraderId={selectedTraderId}
            onTraderSelect={setSelectedTraderId}
          />
        )}
      </main>

      {/* Footer */}
      <footer
        className="mt-16"
        style={{ borderTop: '1px solid #2B3139', background: '#181A20' }}
      >
        <div
          className="max-w-[1920px] mx-auto px-6 py-6 text-center text-sm"
          style={{ color: '#5E6673' }}
        >
          <p>{t('footerTitle', language)}</p>
          <p className="mt-1">{t('footerWarning', language)}</p>
          <div className="mt-4">
            <a
              href="https://github.com/tinkle-community/nofx"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 px-3 py-2 rounded text-sm font-semibold transition-all hover:scale-105"
              style={{
                background: '#1E2329',
                color: '#848E9C',
                border: '1px solid #2B3139',
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.background = '#2B3139'
                e.currentTarget.style.color = '#EAECEF'
                e.currentTarget.style.borderColor = '#F0B90B'
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.background = '#1E2329'
                e.currentTarget.style.color = '#848E9C'
                e.currentTarget.style.borderColor = '#2B3139'
              }}
            >
              <svg
                width="18"
                height="18"
                viewBox="0 0 16 16"
                fill="currentColor"
              >
                <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
              </svg>
              GitHub
            </a>
          </div>
        </div>
      </footer>
    </div>
  )
}

// Trader Details Page Component
function TraderDetailsPage({
  selectedTrader,
  status,
  account,
  positions,
  decisions,
  lastUpdate,
  language,
  traders,
  selectedTraderId,
  onTraderSelect,
}: {
  selectedTrader?: TraderInfo
  traders?: TraderInfo[]
  selectedTraderId?: string
  onTraderSelect: (traderId: string) => void
  status?: SystemStatus
  account?: AccountInfo
  positions?: Position[]
  decisions?: DecisionRecord[]
  stats?: Statistics
  lastUpdate: string
  language: Language
}) {
  const { mutate } = useSWRConfig()
  const [isForceClosing, setIsForceClosing] = useState(false)
  const [isSyncingBalance, setIsSyncingBalance] = useState(false)

  const handleManualClosePositions = async () => {
    if (!selectedTraderId || isForceClosing) return

    // Check current position count
    const positionCount = positions?.length || 0

    // Show appropriate message if no positions
    if (positionCount === 0) {
      alert(t('manualCloseNoPosition', language))
      return
    }

    // Show confirmation dialog with position count
    if (!confirm(t('manualCloseConfirm', language, { count: positionCount }))) {
      return
    }

    try {
      setIsForceClosing(true)
      const response = await fetch(`/api/traders/${selectedTraderId}/close-all`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('auth_token')}`,
          'Content-Type': 'application/json',
        },
      })

      if (!response.ok) {
        throw new Error('Failed to close positions')
      }

      const result = await response.json()
      const closedCount = result.closed_positions || 0

      // Refresh data
      await Promise.all([
        mutate(`positions-${selectedTraderId}`),
        mutate(`account-${selectedTraderId}`),
        mutate(`status-${selectedTraderId}`),
      ])

      // Show success message with count
      alert(t('manualCloseSuccess', language, { count: closedCount }))
    } catch (error) {
      console.error('Failed to close positions:', error)
      alert(t('manualCloseFailed', language))
    } finally {
      setIsForceClosing(false)
    }
  }

  const handleManualSyncBalance = async () => {
    if (!selectedTraderId || isSyncingBalance) return
    try {
      setIsSyncingBalance(true)
      await api.syncBalance(selectedTraderId)
      await Promise.all([
        mutate(`positions-${selectedTraderId}`),
        mutate(`account-${selectedTraderId}`),
        mutate(`status-${selectedTraderId}`),
      ])
      alert(t('manualSyncSuccess', language))
    } catch (error) {
      console.error('Failed to sync balance:', error)
      alert(t('manualSyncFailed', language))
    } finally {
      setIsSyncingBalance(false)
    }
  }

  if (!selectedTrader) {
    return (
      <div className="space-y-6">
        {/* Loading Skeleton - Binance Style */}
        <div className="binance-card p-6 animate-pulse">
          <div className="skeleton h-8 w-48 mb-3"></div>
          <div className="flex gap-4">
            <div className="skeleton h-4 w-32"></div>
            <div className="skeleton h-4 w-24"></div>
            <div className="skeleton h-4 w-28"></div>
          </div>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          {[1, 2, 3, 4].map((i) => (
            <div key={i} className="binance-card p-5 animate-pulse">
              <div className="skeleton h-4 w-24 mb-3"></div>
              <div className="skeleton h-8 w-32"></div>
            </div>
          ))}
        </div>
        <div className="binance-card p-6 animate-pulse">
          <div className="skeleton h-6 w-40 mb-4"></div>
          <div className="skeleton h-64 w-full"></div>
        </div>
      </div>
    )
  }

  return (
    <div>
      {/* Trader Header */}
      <div
        className="mb-6 rounded p-6 animate-scale-in"
        style={{
          background:
            'linear-gradient(135deg, rgba(240, 185, 11, 0.15) 0%, rgba(252, 213, 53, 0.05) 100%)',
          border: '1px solid rgba(240, 185, 11, 0.2)',
          boxShadow: '0 0 30px rgba(240, 185, 11, 0.15)',
        }}
      >
        <div className="flex items-start justify-between mb-3">
          <h2
            className="text-2xl font-bold flex items-center gap-2"
            style={{ color: '#EAECEF' }}
          >
            <span
              className="w-10 h-10 rounded-full flex items-center justify-center text-xl"
              style={{
                background: 'linear-gradient(135deg, #F0B90B 0%, #FCD535 100%)',
              }}
            >
              🤖
            </span>
            {selectedTrader.trader_name}
          </h2>

          {/* Trader Selector */}
          {traders && traders.length > 0 && (
            <div className="flex items-center gap-2">
              <span className="text-sm" style={{ color: '#848E9C' }}>
                {t('switchTrader', language)}:
              </span>
              <select
                value={selectedTraderId}
                onChange={(e) => onTraderSelect(e.target.value)}
                className="rounded px-3 py-2 text-sm font-medium cursor-pointer transition-colors"
                style={{
                  background: '#1E2329',
                  border: '1px solid #2B3139',
                  color: '#EAECEF',
                }}
              >
                {traders.map((trader) => (
                  <option key={trader.trader_id} value={trader.trader_id}>
                    {trader.trader_name}
                  </option>
                ))}
              </select>
            </div>
          )}
        </div>
        <div
          className="flex items-center gap-4 text-sm"
          style={{ color: '#848E9C' }}
        >
          <span>
            AI Model:{' '}
            <span
              className="font-semibold"
              style={{
                color: selectedTrader.ai_model.includes('qwen')
                  ? '#c084fc'
                  : '#60a5fa',
              }}
            >
              {getModelDisplayName(
                selectedTrader.ai_model.split('_').pop() ||
                  selectedTrader.ai_model
              )}
            </span>
          </span>
          {status && (
            <>
              <span>•</span>
              <span>Cycles: {status.call_count}</span>
              <span>•</span>
              <span>Runtime: {status.runtime_minutes} min</span>
            </>
          )}
        </div>
      </div>

      {/* Debug Info */}
  {account && (
    <div
      className="mb-4 p-3 rounded text-xs font-mono flex flex-wrap items-center justify-between gap-3"
      style={{ background: '#1E2329', border: '1px solid #2B3139' }}
    >
      <div style={{ color: '#848E9C' }}>
        🔄 Last Update: {lastUpdate} | Total Equity:{' '}
        {account?.total_equity?.toFixed(2) || '0.00'} | Available:{' '}
        {account?.available_balance?.toFixed(2) || '0.00'} | P&L:{' '}
        {account?.total_pnl?.toFixed(2) || '0.00'} (
        {account?.total_pnl_pct?.toFixed(2) || '0.00'}%)
      </div>
      <button
        onClick={handleManualSyncBalance}
        disabled={isSyncingBalance}
        className="text-xs px-3 py-1 rounded border border-blue-500 text-blue-300 hover:bg-blue-500/10 disabled:opacity-60 disabled:cursor-not-allowed transition-colors"
      >
        {isSyncingBalance
          ? t('manualSyncInProgress', language)
          : t('manualSync', language)}
      </button>
    </div>
  )}

      {/* Account Overview */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-8">
        <StatCard
          title={t('totalEquity', language)}
          value={`${account?.total_equity?.toFixed(2) || '0.00'} USDT`}
          change={account?.total_pnl_pct || 0}
          positive={(account?.total_pnl ?? 0) > 0}
        />
        <StatCard
          title={t('availableBalance', language)}
          value={`${account?.available_balance?.toFixed(2) || '0.00'} USDT`}
          subtitle={`${account?.available_balance && account?.total_equity ? ((account.available_balance / account.total_equity) * 100).toFixed(1) : '0.0'}% ${t('free', language)}`}
        />
        <StatCard
          title={t('totalPnL', language)}
          value={`${account?.total_pnl !== undefined && account.total_pnl >= 0 ? '+' : ''}${account?.total_pnl?.toFixed(2) || '0.00'} USDT`}
          change={account?.total_pnl_pct || 0}
          positive={(account?.total_pnl ?? 0) >= 0}
        />
        <StatCard
          title={t('positions', language)}
          value={`${account?.position_count || 0}`}
          subtitle={`${t('margin', language)}: ${account?.margin_used_pct?.toFixed(1) || '0.0'}%`}
        />
      </div>

      {/* 主要内容区：左右分屏 */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        {/* 左侧：图表 + 持仓 */}
        <div className="space-y-6">
          {/* Equity Chart */}
          <div className="animate-slide-in" style={{ animationDelay: '0.1s' }}>
            <EquityChart traderId={selectedTrader.trader_id} />
          </div>

          {/* Current Positions */}
          <div
            className="binance-card p-6 animate-slide-in"
            style={{ animationDelay: '0.15s' }}
          >
            <div className="flex items-center justify-between mb-5 gap-3">
              <h2
                className="text-xl font-bold flex items-center gap-2"
                style={{ color: '#EAECEF' }}
              >
                📈 {t('currentPositions', language)}
              </h2>
              <div className="flex items-center gap-3">
                {positions && positions.length > 0 && (
                  <div
                    className="text-xs px-3 py-1 rounded"
                    style={{
                      background: 'rgba(240, 185, 11, 0.1)',
                      color: '#F0B90B',
                      border: '1px solid rgba(240, 185, 11, 0.2)',
                    }}
                  >
                    {positions.length} {t('active', language)}
                  </div>
                )}
                {positions && positions.length > 0 && (
                  <button
                    onClick={handleManualClosePositions}
                    disabled={isForceClosing}
                    className="text-xs px-3 py-1 rounded border border-red-500 text-red-400 hover:bg-red-500/10 disabled:opacity-60 disabled:cursor-not-allowed transition-colors"
                  >
                    {isForceClosing
                      ? t('manualCloseInProgress', language)
                      : t('manualClose', language)}
                  </button>
                )}
              </div>
            </div>
            {positions && positions.length > 0 ? (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="text-left border-b border-gray-800">
                    <tr>
                      <th className="pb-3 font-semibold text-gray-400">
                        {t('symbol', language)}
                      </th>
                      <th className="pb-3 font-semibold text-gray-400">
                        {t('side', language)}
                      </th>
                      <th className="pb-3 font-semibold text-gray-400">
                        {t('entryPrice', language)}
                      </th>
                      <th className="pb-3 font-semibold text-gray-400">
                        {t('markPrice', language)}
                      </th>
                      <th className="pb-3 font-semibold text-gray-400">
                        {t('quantity', language)}
                      </th>
                      <th className="pb-3 font-semibold text-gray-400">
                        {t('positionValue', language)}
                      </th>
                      <th className="pb-3 font-semibold text-gray-400">
                        {t('leverage', language)}
                      </th>
                      <th className="pb-3 font-semibold text-gray-400">
                        {t('unrealizedPnL', language)}
                      </th>
                      <th className="pb-3 font-semibold text-gray-400">
                        {t('liqPrice', language)}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {positions.map((pos, i) => (
                      <tr
                        key={i}
                        className="border-b border-gray-800 last:border-0"
                      >
                        <td className="py-3 font-mono font-semibold">
                          {pos.symbol}
                        </td>
                        <td className="py-3">
                          <span
                            className="px-2 py-1 rounded text-xs font-bold"
                            style={
                              pos.side === 'long'
                                ? {
                                    background: 'rgba(14, 203, 129, 0.1)',
                                    color: '#0ECB81',
                                  }
                                : {
                                    background: 'rgba(246, 70, 93, 0.1)',
                                    color: '#F6465D',
                                  }
                            }
                          >
                            {t(
                              pos.side === 'long' ? 'long' : 'short',
                              language
                            )}
                          </span>
                        </td>
                        <td
                          className="py-3 font-mono"
                          style={{ color: '#EAECEF' }}
                        >
                          {pos.entry_price.toFixed(4)}
                        </td>
                        <td
                          className="py-3 font-mono"
                          style={{ color: '#EAECEF' }}
                        >
                          {pos.mark_price.toFixed(4)}
                        </td>
                        <td
                          className="py-3 font-mono"
                          style={{ color: '#EAECEF' }}
                        >
                          {pos.quantity.toFixed(4)}
                        </td>
                        <td
                          className="py-3 font-mono font-bold"
                          style={{ color: '#EAECEF' }}
                        >
                          {(pos.quantity * pos.mark_price).toFixed(2)} USDT
                        </td>
                        <td
                          className="py-3 font-mono"
                          style={{ color: '#F0B90B' }}
                        >
                          {pos.leverage}x
                        </td>
                        <td className="py-3 font-mono">
                          <span
                            style={{
                              color:
                                pos.unrealized_pnl >= 0 ? '#0ECB81' : '#F6465D',
                              fontWeight: 'bold',
                            }}
                          >
                            {pos.unrealized_pnl >= 0 ? '+' : ''}
                            {pos.unrealized_pnl.toFixed(2)} (
                            {pos.unrealized_pnl_pct.toFixed(2)}%)
                          </span>
                        </td>
                        <td
                          className="py-3 font-mono"
                          style={{ color: '#848E9C' }}
                        >
                          {pos.liquidation_price.toFixed(4)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <div className="text-center py-16" style={{ color: '#848E9C' }}>
                <div className="text-6xl mb-4 opacity-50">📊</div>
                <div className="text-lg font-semibold mb-2">
                  {t('noPositions', language)}
                </div>
                <div className="text-sm">
                  {t('noActivePositions', language)}
                </div>
              </div>
            )}
          </div>
        </div>
        {/* 左侧结束 */}

        {/* 右侧：Recent Decisions - 卡片容器 */}
        <div
          className="binance-card p-6 animate-slide-in h-fit lg:sticky lg:top-24 lg:max-h-[calc(100vh-120px)]"
          style={{ animationDelay: '0.2s' }}
        >
          {/* 标题 */}
          <div
            className="flex items-center gap-3 mb-5 pb-4 border-b"
            style={{ borderColor: '#2B3139' }}
          >
            <div
              className="w-10 h-10 rounded-xl flex items-center justify-center text-xl"
              style={{
                background: 'linear-gradient(135deg, #6366F1 0%, #8B5CF6 100%)',
                boxShadow: '0 4px 14px rgba(99, 102, 241, 0.4)',
              }}
            >
              🧠
            </div>
            <div>
              <h2 className="text-xl font-bold" style={{ color: '#EAECEF' }}>
                {t('recentDecisions', language)}
              </h2>
              {decisions && decisions.length > 0 && (
                <div className="text-xs" style={{ color: '#848E9C' }}>
                  {t('lastCycles', language, { count: decisions.length })}
                </div>
              )}
            </div>
          </div>

          {/* 决策列表 - 可滚动 */}
          <div
            className="space-y-4 overflow-y-auto pr-2"
            style={{ maxHeight: 'calc(100vh - 280px)' }}
          >
            {decisions && decisions.length > 0 ? (
              decisions.map((decision, i) => (
                <DecisionCard key={i} decision={decision} language={language} />
              ))
            ) : (
              <div className="py-16 text-center">
                <div className="text-6xl mb-4 opacity-30">🧠</div>
                <div
                  className="text-lg font-semibold mb-2"
                  style={{ color: '#EAECEF' }}
                >
                  {t('noDecisionsYet', language)}
                </div>
                <div className="text-sm" style={{ color: '#848E9C' }}>
                  {t('aiDecisionsWillAppear', language)}
                </div>
              </div>
            )}
          </div>
        </div>
        {/* 右侧结束 */}
      </div>

      {/* AI Learning & Performance Analysis */}
      <div className="mb-6 animate-slide-in" style={{ animationDelay: '0.3s' }}>
        <AILearning traderId={selectedTrader.trader_id} />
      </div>
    </div>
  )
}

// Stat Card Component - Binance Style Enhanced
function StatCard({
  title,
  value,
  change,
  positive,
  subtitle,
}: {
  title: string
  value: string
  change?: number
  positive?: boolean
  subtitle?: string
}) {
  return (
    <div className="stat-card animate-fade-in">
      <div
        className="text-xs mb-2 mono uppercase tracking-wider"
        style={{ color: '#848E9C' }}
      >
        {title}
      </div>
      <div
        className="text-2xl font-bold mb-1 mono"
        style={{ color: '#EAECEF' }}
      >
        {value}
      </div>
      {change !== undefined && (
        <div className="flex items-center gap-1">
          <div
            className="text-sm mono font-bold"
            style={{ color: positive ? '#0ECB81' : '#F6465D' }}
          >
            {positive ? '▲' : '▼'} {positive ? '+' : ''}
            {change.toFixed(2)}%
          </div>
        </div>
      )}
      {subtitle && (
        <div className="text-xs mt-2 mono" style={{ color: '#848E9C' }}>
          {subtitle}
        </div>
      )}
    </div>
  )
}

// Decision Card Component with CoT Trace - Binance Style
function DecisionCard({
  decision,
  language,
}: {
  decision: DecisionRecord
  language: Language
}) {
  const [showInputPrompt, setShowInputPrompt] = useState(false)
  const [showCoT, setShowCoT] = useState(false)

  return (
    <div
      className="rounded p-5 transition-all duration-300 hover:translate-y-[-2px]"
      style={{
        border: '1px solid #2B3139',
        background: '#1E2329',
        boxShadow: '0 2px 8px rgba(0, 0, 0, 0.3)',
      }}
    >
      {/* Header */}
      <div className="flex items-start justify-between mb-3">
        <div>
          <div className="font-semibold" style={{ color: '#EAECEF' }}>
            {t('cycle', language)} #{decision.cycle_number}
          </div>
          <div className="text-xs" style={{ color: '#848E9C' }}>
            {new Date(decision.timestamp).toLocaleString()}
          </div>
        </div>
        <div
          className="px-3 py-1 rounded text-xs font-bold"
          style={
            decision.success
              ? { background: 'rgba(14, 203, 129, 0.1)', color: '#0ECB81' }
              : { background: 'rgba(246, 70, 93, 0.1)', color: '#F6465D' }
          }
        >
          {t(decision.success ? 'success' : 'failed', language)}
        </div>
      </div>

      {/* Input Prompt - Collapsible */}
      {decision.input_prompt && (
        <div className="mb-3">
          <button
            onClick={() => setShowInputPrompt(!showInputPrompt)}
            className="flex items-center gap-2 text-sm transition-colors"
            style={{ color: '#60a5fa' }}
          >
            <span className="font-semibold">
              📥 {t('inputPrompt', language)}
            </span>
            <span className="text-xs">
              {showInputPrompt
                ? t('collapse', language)
                : t('expand', language)}
            </span>
          </button>
          {showInputPrompt && (
            <div
              className="mt-2 rounded p-4 text-sm font-mono whitespace-pre-wrap max-h-96 overflow-y-auto"
              style={{
                background: '#0B0E11',
                border: '1px solid #2B3139',
                color: '#EAECEF',
              }}
            >
              {decision.input_prompt}
            </div>
          )}
        </div>
      )}

      {/* AI Chain of Thought - Collapsible */}
      {decision.cot_trace && (
        <div className="mb-3">
          <button
            onClick={() => setShowCoT(!showCoT)}
            className="flex items-center gap-2 text-sm transition-colors"
            style={{ color: '#F0B90B' }}
          >
            <span className="font-semibold">
              📤 {t('aiThinking', language)}
            </span>
            <span className="text-xs">
              {showCoT ? t('collapse', language) : t('expand', language)}
            </span>
          </button>
          {showCoT && (
            <div
              className="mt-2 rounded p-4 text-sm font-mono whitespace-pre-wrap max-h-96 overflow-y-auto"
              style={{
                background: '#0B0E11',
                border: '1px solid #2B3139',
                color: '#EAECEF',
              }}
            >
              {decision.cot_trace}
            </div>
          )}
        </div>
      )}

      {/* Decisions Actions */}
      {decision.decisions && decision.decisions.length > 0 && (
        <div className="space-y-2 mb-3">
          {decision.decisions.map((action, j) => (
            <div
              key={j}
              className="flex items-center gap-2 text-sm rounded px-3 py-2"
              style={{ background: '#0B0E11' }}
            >
              <span
                className="font-mono font-bold"
                style={{ color: '#EAECEF' }}
              >
                {action.symbol}
              </span>
              <span
                className="px-2 py-0.5 rounded text-xs font-bold"
                style={
                  action.action.includes('open')
                    ? {
                        background: 'rgba(96, 165, 250, 0.1)',
                        color: '#60a5fa',
                      }
                    : {
                        background: 'rgba(240, 185, 11, 0.1)',
                        color: '#F0B90B',
                      }
                }
              >
                {action.action}
              </span>
              {action.leverage > 0 && (
                <span style={{ color: '#F0B90B' }}>{action.leverage}x</span>
              )}
              {action.price > 0 && (
                <span
                  className="font-mono text-xs"
                  style={{ color: '#848E9C' }}
                >
                  @{action.price.toFixed(4)}
                </span>
              )}
              <span style={{ color: action.success ? '#0ECB81' : '#F6465D' }}>
                {action.success ? '✓' : '✗'}
              </span>
              {action.error && (
                <span className="text-xs ml-2" style={{ color: '#F6465D' }}>
                  {action.error}
                </span>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Account State Summary */}
      {decision.account_state && (
        <div
          className="flex gap-4 text-xs mb-3 rounded px-3 py-2"
          style={{ background: '#0B0E11', color: '#848E9C' }}
        >
          <span>
            净值: {decision.account_state.total_balance.toFixed(2)} USDT
          </span>
          <span>
            可用: {decision.account_state.available_balance.toFixed(2)} USDT
          </span>
          <span>
            保证金率: {decision.account_state.margin_used_pct.toFixed(1)}%
          </span>
          <span>持仓: {decision.account_state.position_count}</span>
          <span
            style={{
              color:
                decision.candidate_coins &&
                decision.candidate_coins.length === 0
                  ? '#F6465D'
                  : '#848E9C',
            }}
          >
            {t('candidateCoins', language)}:{' '}
            {decision.candidate_coins?.length || 0}
          </span>
        </div>
      )}

      {/* Candidate Coins Warning */}
      {decision.candidate_coins && decision.candidate_coins.length === 0 && (
        <div
          className="text-sm rounded px-4 py-3 mb-3 flex items-start gap-3"
          style={{
            background: 'rgba(246, 70, 93, 0.1)',
            border: '1px solid rgba(246, 70, 93, 0.3)',
            color: '#F6465D',
          }}
        >
          <AlertTriangle size={16} className="flex-shrink-0 mt-0.5" />
          <div className="flex-1">
            <div className="font-semibold mb-1">
              ⚠️ {t('candidateCoinsZeroWarning', language)}
            </div>
            <div className="text-xs space-y-1" style={{ color: '#848E9C' }}>
              <div>{t('possibleReasons', language)}</div>
              <ul className="list-disc list-inside space-y-0.5 ml-2">
                <li>{t('coinPoolApiNotConfigured', language)}</li>
                <li>{t('apiConnectionTimeout', language)}</li>
                <li>{t('noCustomCoinsAndApiFailed', language)}</li>
              </ul>
              <div className="mt-2">
                <strong>{t('solutions', language)}</strong>
              </div>
              <ul className="list-disc list-inside space-y-0.5 ml-2">
                <li>{t('setCustomCoinsInConfig', language)}</li>
                <li>{t('orConfigureCorrectApiUrl', language)}</li>
                <li>{t('orDisableCoinPoolOptions', language)}</li>
              </ul>
            </div>
          </div>
        </div>
      )}

      {/* Execution Logs */}
      {decision.execution_log && decision.execution_log.length > 0 && (
        <div className="space-y-1">
          {decision.execution_log.map((log, k) => (
            <div
              key={k}
              className="text-xs font-mono"
              style={{
                color:
                  log.includes('✓') || log.includes('成功')
                    ? '#0ECB81'
                    : '#F6465D',
              }}
            >
              {log}
            </div>
          ))}
        </div>
      )}

      {/* Error Message */}
      {decision.error_message && (
        <div
          className="text-sm rounded px-3 py-2 mt-3"
          style={{ color: '#F6465D', background: 'rgba(246, 70, 93, 0.1)' }}
        >
          ❌ {decision.error_message}
        </div>
      )}
    </div>
  )
}

// Wrap App with providers
export default function AppWithProviders() {
  return (
    <LanguageProvider>
      <AuthProvider>
        <App />
      </AuthProvider>
    </LanguageProvider>
  )
}
