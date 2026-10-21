import React from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
// After the base table, so a theme's block wins over it.
import './themes.css'
import App from './App'
import {Recover} from './Recover'
import {installDropGuard} from './dropGuard'

// A file or link dropped where no page takes it must not open in place of
// the app.
installDropGuard(window)

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        {/* The last catch: a fault in the shell itself would otherwise
            leave a blank window. */}
        <Recover subject="Agentry Desktop" recoverLabel="Reload" onRecover={() => window.location.reload()}
                 hint="Reload starts it again from the beginning.">
            <App/>
        </Recover>
    </React.StrictMode>
)
