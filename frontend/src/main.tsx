import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import '@fontsource-variable/space-grotesk'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/600.css'
import './styles.css'
import App from './App'
import { BrandingProvider } from './branding'
import { DialogProvider } from './components/Dialog'
import { initMode } from './components/ThemeToggle'

initMode()

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <BrandingProvider>
        <DialogProvider>
          <App />
        </DialogProvider>
      </BrandingProvider>
    </BrowserRouter>
  </React.StrictMode>,
)
