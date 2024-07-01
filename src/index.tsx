import React from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { Auth0Provider } from '@auth0/auth0-react';
import App from './App';
import './styles.css';
import { AppProvider } from './contexts/AppContext'; // Corrected path

const container = document.getElementById('root');
const root = createRoot(container!); // Use createRoot instead of ReactDOM.render

root.render(
  <React.StrictMode>
    <BrowserRouter>
      <Auth0Provider
        domain="dev-qg1ftdys736bk5i3.us.auth0.com"
        clientId="Be5vsLunFvpzPf4xfXtaMxrZUVBjjNPO"
        redirectUri={window.location.origin}
        cacheLocation="localstorage"
        onRedirectCallback={(appState) => {
          window.history.replaceState(
            {},
            document.title,
            appState?.returnTo || window.location.pathname
          );
        }}
      >
        <AppProvider>
          <App />
        </AppProvider>
      </Auth0Provider>
    </BrowserRouter>
  </React.StrictMode>
);
