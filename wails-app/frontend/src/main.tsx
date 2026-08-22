import React from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
import App from './App'
import {AppendLog} from '../wailsjs/go/main/App'

/**
 * Without this, a single render error leaves an empty window with no clue what
 * happened. The message is shown and written to the auto-saved log instead.
 */
class ErrorBoundary extends React.Component<{children: React.ReactNode}, {message: string}> {
    state = {message: ''}

    static getDerivedStateFromError(error: unknown) {
        return {message: error instanceof Error ? error.message : String(error)}
    }

    componentDidCatch(error: unknown, info: React.ErrorInfo) {
        const detail = error instanceof Error ? `${error.message}\n${error.stack ?? ''}` : String(error)
        AppendLog(`Giao diện lỗi: ${detail}\n${info.componentStack ?? ''}`).catch(() => undefined)
    }

    render() {
        if (!this.state.message) return this.props.children
        return <div className="crash-screen">
            <h1>Giao diện gặp lỗi</h1>
            <p>Chi tiết đã được ghi vào nhật ký trong thư mục cấu hình của ứng dụng.</p>
            <pre>{this.state.message}</pre>
            <button onClick={() => window.location.reload()}>Tải lại giao diện</button>
        </div>
    }
}

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        <ErrorBoundary>
            <App/>
        </ErrorBoundary>
    </React.StrictMode>
)
