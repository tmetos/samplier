import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, NavLink, Navigate, Route, Routes } from 'react-router-dom'
import 'maplibre-gl/dist/maplibre-gl.css'

import { BrandMark } from './components/BrandMark'
import './styles.css'
import { Dashboard } from './pages/Dashboard'
import { MapPage } from './pages/MapPage'
import { ProjectsPage } from './pages/ProjectsPage'
import { ResultsPage } from './pages/ResultsPage'
import { ActionLevelsPage } from './pages/ActionLevelsPage'
import { ImportPage } from './pages/ImportPage'
import { ExportsPage } from './pages/ExportsPage'

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, retry: 1 } },
})

const nav = [
  { to: '/dashboard', label: 'Dashboard' },
  { to: '/map', label: 'Map' },
  { to: '/projects', label: 'Projects' },
  { to: '/results', label: 'Results' },
  { to: '/action-levels', label: 'Action Levels' },
  { to: '/import', label: 'EDD Import' },
  { to: '/exports', label: 'Reports & Exports' },
]

function Shell() {
  return (
    <div className="app">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-mark">
            <BrandMark size={32} />
          </span>
          <div>
            <div className="brand-name">Samplier</div>
            <div className="brand-tag">Environmental data</div>
          </div>
        </div>
        <nav>
          {nav.map((n) => (
            <NavLink key={n.to} to={n.to} className={({ isActive }) => (isActive ? 'nav active' : 'nav')}>
              {n.label}
            </NavLink>
          ))}
        </nav>
        <footer className="sidebar-footer">
          <a href="/ogc/" target="_blank" rel="noreferrer">
            OGC API
          </a>
          <span>AGPL-3.0</span>
        </footer>
      </aside>

      <main className="content">
        <Routes>
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/map" element={<MapPage />} />
          <Route path="/projects" element={<ProjectsPage />} />
          <Route path="/results" element={<ResultsPage />} />
          <Route path="/action-levels" element={<ActionLevelsPage />} />
          <Route path="/import" element={<ImportPage />} />
          <Route path="/exports" element={<ExportsPage />} />
          <Route path="*" element={<Navigate to="/dashboard" replace />} />
        </Routes>
      </main>
    </div>
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Shell />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
