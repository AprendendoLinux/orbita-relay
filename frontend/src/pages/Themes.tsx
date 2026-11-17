import { useCallback, useEffect, useState } from 'react'
import { api, type Theme } from '../api'
import { applyTokens, useBranding } from '../branding'

const colorKeys: [string, string][] = [
  ['bg', 'Fundo'], ['bg2', 'Fundo (gradiente)'], ['surface', 'Painéis (vidro)'], ['border', 'Bordas'],
  ['text', 'Texto'], ['muted', 'Texto secundário'], ['primary', 'Cor primária'], ['secondary', 'Cor secundária'],
  ['accent', 'Destaque'], ['success', 'Sucesso'], ['warning', 'Alerta'], ['danger', 'Erro'], ['glow', 'Brilho (glow)'],
]

const hex6 = (v: string) => /^#[0-9a-fA-F]{6}$/.test(v)

export default function Themes() {
  const { refresh, branding } = useBranding()
  const [themes, setThemes] = useState<Theme[]>([])
  const [active, setActive] = useState('')
  const [edit, setEdit] = useState<Theme | null>(null)
  const [msg, setMsg] = useState<{ t: 'ok' | 'err'; s: string } | null>(null)

  const load = useCallback(async () => {
    const r = await api<{ themes: Theme[]; active: string }>('/api/admin/themes')
    setThemes(r.themes)
    setActive(r.active)
  }, [])
  useEffect(() => { load().catch((e) => setMsg({ t: 'err', s: e.message })) }, [load])

  const act = async (fn: () => Promise<unknown>, ok: string) => {
    try {
      await fn()
      setMsg({ t: 'ok', s: ok })
      await load()
      await refresh()
    } catch (e) {
      setMsg({ t: 'err', s: (e as Error).message })
    }
  }

  const duplicate = (t: Theme) => {
    const name = prompt('Nome do novo tema', `${t.name} (cópia)`)
    if (name) act(() => api('/api/admin/themes', { method: 'POST', body: { name, from: t.slug } }), 'Tema criado')
  }

  const setToken = (k: string, v: string) => {
    if (!edit) return
    const t = { ...edit, tokens: { ...edit.tokens, [k]: v } }
    setEdit(t)
    applyTokens(t.tokens)
  }

  const saveEdit = () =>
    edit && act(async () => {
      await api(`/api/admin/themes/${edit.slug}`, { method: 'PUT', body: { name: edit.name, tokens: edit.tokens } })
      setEdit(null)
    }, 'Tema salvo')

  const cancelEdit = () => {
    setEdit(null)
    if (branding) applyTokens(branding.theme.tokens)
  }

  const upload = (slug: string, file?: File) => {
    if (!file) return
    const form = new FormData()
    form.append('logo', file)
    act(() => api(`/api/admin/themes/${slug}/logo`, { method: 'POST', form }), 'Logotipo atualizado')
  }

  if (edit)
    return (
      <>
        <div className="topbar"><h1>Editar tema</h1></div>
        <p className="muted small">As alterações são pré-visualizadas ao vivo. Salve para persistir.</p>
        {msg && <div className={`alert ${msg.t}`} style={{ margin: '10px 0' }}>{msg.s}</div>}
        <div className="glass card stack">
          <label className="f">Nome<input value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} /></label>
          <div className="form-grid">
            {colorKeys.map(([k, label]) => (
              <label className="f" key={k}>{label}
                <div className="row" style={{ flexWrap: 'nowrap' }}>
                  {hex6(edit.tokens[k]) && <input type="color" value={edit.tokens[k]} onChange={(e) => setToken(k, e.target.value)} aria-label={`Seletor ${label}`} />}
                  <input value={edit.tokens[k]} onChange={(e) => setToken(k, e.target.value)} className="mono" />
                </div>
              </label>
            ))}
            <label className="f">Arredondamento (px)<input type="number" min={0} max={40} value={edit.tokens.radius} onChange={(e) => setToken('radius', e.target.value)} /></label>
            <label className="f">Desfoque do vidro (px)<input type="number" min={0} max={40} value={edit.tokens.blur} onChange={(e) => setToken('blur', e.target.value)} /></label>
            <label className="f">Fonte
              <select value={edit.tokens.font} onChange={(e) => setToken('font', e.target.value)}>
                <option value="space-grotesk">Space Grotesk</option>
                <option value="inter">Inter</option>
                <option value="jetbrains-mono">JetBrains Mono</option>
              </select>
            </label>
          </div>
          <div className="row">
            <button className="primary" onClick={saveEdit}>Salvar tema</button>
            <button onClick={cancelEdit}>Cancelar</button>
          </div>
        </div>
      </>
    )

  return (
    <>
      <div className="topbar"><h1>Temas e identidade visual</h1></div>
      {msg && <div className={`alert ${msg.t}`} role="status" style={{ marginBottom: 14 }}>{msg.s}</div>}
      <div className="grid cols-2">
        {themes.map((t) => (
          <div key={t.slug} className={`glass theme-card ${t.slug === active ? 'active' : ''}`}>
            <div className="row spread">
              <h3 style={{ margin: 0 }}>{t.name}</h3>
              <div className="row">
                {t.builtin && <span className="badge b-info">padrão</span>}
                {t.slug === active && <span className="badge b-ok">ativo</span>}
              </div>
            </div>
            <div className="theme-prev" style={{ background: `linear-gradient(135deg, ${t.tokens.bg}, ${t.tokens.bg2})`, display: 'flex', alignItems: 'center', gap: 8, padding: '0 12px' }}>
              {['primary', 'secondary', 'accent', 'success'].map((k) => <span key={k} className="swatch" style={{ background: t.tokens[k], boxShadow: `0 0 10px ${t.tokens.glow}` }} />)}
              {t.logo && <img src={t.logo} alt="" style={{ height: 36, marginLeft: 'auto' }} />}
            </div>
            <div className="row">
              {t.slug !== active && <button className="sm primary" onClick={() => act(() => api(`/api/admin/themes/${t.slug}/activate`, { method: 'POST' }), 'Tema ativado')}>Ativar</button>}
              <button className="sm" onClick={() => duplicate(t)}>Duplicar</button>
              {!t.builtin && (
                <>
                  <button className="sm" onClick={() => setEdit(t)}>Editar cores</button>
                  <label className="btn sm" style={{ cursor: 'pointer' }}>
                    Logotipo
                    <input type="file" accept="image/png,image/jpeg,image/webp,image/svg+xml" hidden onChange={(e) => upload(t.slug, e.target.files?.[0])} />
                  </label>
                  {t.logo && <button className="sm" onClick={() => act(() => api(`/api/admin/themes/${t.slug}/logo`, { method: 'DELETE' }), 'Logotipo removido')}>Remover logo</button>}
                  {t.slug !== active && (
                    <button className="sm danger" onClick={() => confirm(`Excluir o tema "${t.name}"?`) && act(() => api(`/api/admin/themes/${t.slug}`, { method: 'DELETE' }), 'Tema excluído')}>Excluir</button>
                  )}
                </>
              )}
            </div>
            {t.builtin && <span className="help">Duplique para personalizar cores e logotipo.</span>}
          </div>
        ))}
      </div>
    </>
  )
}
