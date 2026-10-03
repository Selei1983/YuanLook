import {createBrowserRouter, Navigate} from 'react-router-dom';
import {type ComponentType, lazy, type LazyExoticComponent, Suspense} from 'react';
import PrivateRoute from '@/components/PrivateRoute';

const RegisterPage = lazy(() => import('@/pages/Login/Register'));
const AccountsPage = lazy(() => import('@/pages/Accounts'));
const PasswordPage = lazy(() => import('@/pages/Accounts/Password'));
const LoginPage = lazy(() => import('@/pages/Login'));
const GitHubCallbackPage = lazy(() => import('@/pages/Login/GitHubCallback'));
const OIDCCallbackPage = lazy(() => import('@/pages/Login/OIDCCallback'));
const AdminLayout = lazy(() => import('@/layouts/AdminLayout'));
const AgentListPage = lazy(() => import('@/pages/Agents/AgentList'));
const AgentDetailPage = lazy(() => import('@/pages/Agents/AgentDetail'));
const AgentInstallOneClickPage = lazy(() => import('@/pages/Agents/AgentInstallOneClick'));
const AgentInstallManualPage = lazy(() => import('@/pages/Agents/AgentInstallManual'));
const ApiKeyListPage = lazy(() => import('@/pages/ApiKeys/ApiKeyList'));
const ManageApiKeyListPage = lazy(() => import('@/pages/ManageApiKeys/ManageApiKeyList'));
const SettingsPage = lazy(() => import('@/pages/Settings'));
const MonitorListPage = lazy(() => import('@/pages/Monitors/MonitorList'));
const DDNSPage = lazy(() => import('@/pages/DDNS'));
const AlertRecordListPage = lazy(() => import('@/pages/AlertRecords'));

const lazyLoad = (Component: LazyExoticComponent<ComponentType<any>>) => (
    <Suspense fallback={
        <div className="flex min-h-[60vh] items-center justify-center gap-2.5 text-[13px] text-slate-500" role="status" aria-live="polite">
            <span className="size-[18px] animate-spin rounded-full border-2 border-[#dbe4ee] border-t-[#1677ff]"/>
            <span>页面加载中</span>
        </div>
    }>
        <Component/>
    </Suspense>
);

export default createBrowserRouter([
    {path: '/admin/register', element: lazyLoad(RegisterPage)},
    {path: '/admin/login', element: lazyLoad(LoginPage)},
    {path: '/admin/github/callback', element: lazyLoad(GitHubCallbackPage)},
    {path: '/admin/oidc/callback', element: lazyLoad(OIDCCallbackPage)},
    {
        path: '/admin',
        element: <PrivateRoute><AdminLayout/></PrivateRoute>,
        children: [
            {index: true, element: <Navigate to="/admin/agents" replace/>},
            {path: 'agents', element: lazyLoad(AgentListPage)},
            {path: 'agents/:id', element: lazyLoad(AgentDetailPage)},
            {path: 'agents-install', element: <Navigate to="/admin/agents-install/one-click" replace/>},
            {path: 'agents-install/one-click', element: lazyLoad(AgentInstallOneClickPage)},
            {path: 'agents-install/manual', element: lazyLoad(AgentInstallManualPage)},
            {path: 'api-keys', element: lazyLoad(ApiKeyListPage)},
            {path: 'manage-api-keys', element: lazyLoad(ManageApiKeyListPage)},
            {path: 'monitors', element: lazyLoad(MonitorListPage)},
            {path: 'ddns', element: lazyLoad(DDNSPage)},
            {path: 'alert-records', element: lazyLoad(AlertRecordListPage)},
            {path: 'accounts', element: lazyLoad(AccountsPage)},
            {path: 'account/password', element: lazyLoad(PasswordPage)},
            {path: 'settings', element: lazyLoad(SettingsPage)},
        ],
    },
    {path: '*', element: <Navigate to="/admin" replace/>},
]);
