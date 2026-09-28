import React from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
import App from './App'
import {Recover} from './Recover'

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        {/* The last catch: a fault in the shell itself would otherwise
            leave a blank window (agentry-desktop#351). */}
        <Recover what="Agentry Desktop" recoverLabel="Reload" onRecover={() => window.location.reload()}
                 hint="Reload opens it again where it starts.">
            <App/>
        </Recover>
    </React.StrictMode>
)
