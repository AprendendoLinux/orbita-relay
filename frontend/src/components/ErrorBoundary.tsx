import { Component, type ReactNode } from 'react'

type S = { error: Error | null }

export default class ErrorBoundary extends Component<{ children: ReactNode }, S> {
  state: S = { error: null }

  static getDerivedStateFromError(error: Error): S {
    return { error }
  }

  render() {
    if (!this.state.error) return this.props.children
    return (
      <div className="center">
        <div className="auth glass stack" role="alert">
          <h1>Algo deu errado</h1>
          <p className="muted small" style={{ textAlign: 'center' }}>Ocorreu um erro ao exibir esta tela. Recarregue a página; se persistir, informe a mensagem abaixo.</p>
          <code className="block mono" style={{ wordBreak: 'break-word' }}>{this.state.error.message}</code>
          <div className="row" style={{ justifyContent: 'center' }}>
            <button className="primary" onClick={() => window.location.reload()}>Recarregar</button>
            <button onClick={() => window.location.assign('/')}>Ir para o início</button>
          </div>
        </div>
      </div>
    )
  }
}
