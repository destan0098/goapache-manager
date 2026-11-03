export interface User {
    id: number
    username: string
    role: string
}

export interface VHost {
    domain: string
    port: number
    file: string
    line: number
    default: boolean
}

export interface Backup {
    id: string
    type: string
    size: number
    modified: string
}

export interface APIResponse {
    success: boolean
    output?: string
    error?: string
    message?: string
}
export interface VHostDetails {
    domain: string
    document_root: string
    server_aliases: string[]
}

function getCSRFToken(): string {
    const item = document.cookie
        .split('; ')
        .find((cookie) => cookie.startsWith('goapache_csrf='))

    return item
        ? decodeURIComponent(item.substring('goapache_csrf='.length))
        : ''
}

async function request<T>(
    path: string,
    options: RequestInit = {},
): Promise<T> {
    const method = (options.method || 'GET').toUpperCase()
    const headers = new Headers(options.headers)

    headers.set('Accept', 'application/json')

    if (options.body) {
        headers.set('Content-Type', 'application/json')
    }

    if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
        const csrf = getCSRFToken()
        if (csrf) headers.set('X-CSRF-Token', csrf)
    }

    const response = await fetch(path, {
        ...options,
        method,
        headers,
        credentials: 'include',
    })

    const raw = await response.text()

    if (!response.ok) {
        let message = raw || `HTTP ${response.status}`

        try {
            const parsed = JSON.parse(raw)
            message = parsed.error || parsed.message || message
        } catch {
            // Backend may return plain-text errors.
        }

        throw new Error(message)
    }

    if (!raw) return undefined as T

    return JSON.parse(raw) as T
}

export const api = {
    login: (username: string, password: string) =>
        request<{ success: boolean; csrf: string }>('/api/v1/auth/login', {
            method: 'POST',
            body: JSON.stringify({username, password}),
        }),

    logout: () =>
        request<{ success: boolean }>('/api/v1/auth/logout', {
            method: 'POST',
        }),

    me: () => request<User>('/api/v1/auth/me'),

    status: () =>
        request<APIResponse>('/api/v1/apache/status'),

    configTest: () =>
        request<APIResponse>('/api/v1/apache/configtest', {
            method: 'POST',
        }),

    reload: () =>
        request<APIResponse>('/api/v1/apache/reload', {
            method: 'POST',
        }),
    getVHost: (domain: string) =>
        request<{ success: boolean; vhost: VHostDetails }>(
            `/api/v1/apache/vhosts/${encodeURIComponent(domain)}`
        ),

    restart: () =>
        request<APIResponse>('/api/v1/apache/restart', {
            method: 'POST',
        }),

    vhosts: () =>
        request<{ success: boolean; count: number; vhosts: VHost[] }>(
            '/api/v1/apache/vhosts',
        ),

    createVHost: (domain: string) =>
        request<APIResponse>('/api/v1/apache/vhosts', {
            method: 'POST',
            body: JSON.stringify({domain}),
        }),
    editVHost: (
        domain: string,
        documentRoot: string,
        serverAliases: string[],
    ) =>
        request<APIResponse>(
            `/api/v1/apache/vhosts/${encodeURIComponent(domain)}`,
            {
                method: 'PUT',
                body: JSON.stringify({
                    document_root: documentRoot,
                    server_aliases: serverAliases,
                }),
            },
        ),

    enableVHost: (domain: string) =>
        request<APIResponse>(
            `/api/v1/apache/vhosts/${encodeURIComponent(domain)}/enable`,
            {method: 'POST'},
        ),

    disableVHost: (domain: string) =>
        request<APIResponse>(
            `/api/v1/apache/vhosts/${encodeURIComponent(domain)}/disable`,
            {method: 'POST'},
        ),

    deleteVHost: (domain: string) =>
        request<APIResponse>(
            `/api/v1/apache/vhosts/${encodeURIComponent(domain)}`,
            {method: 'DELETE'},
        ),

    backups: (domain: string) =>
        request<{
            success: boolean
            domain: string
            count: number
            backups: Backup[]
        }>(
            `/api/v1/apache/vhosts/${encodeURIComponent(domain)}/backups`,
        ),

    restoreBackup: (domain: string, id: string) =>
        request<APIResponse>(
            `/api/v1/apache/vhosts/${encodeURIComponent(domain)}/backups/${encodeURIComponent(id)}/restore`,
            {method: 'POST'},
        ),

    deleteBackup: (domain: string, id: string) =>
        request<APIResponse>(
            `/api/v1/apache/vhosts/${encodeURIComponent(domain)}/backups/${encodeURIComponent(id)}`,
            {method: 'DELETE'},
        ),
}
