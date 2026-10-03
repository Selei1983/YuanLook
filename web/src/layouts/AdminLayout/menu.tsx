import type {JSX} from 'react';
import {Activity, AlertTriangle, Globe, Key, Server, Settings, Users} from 'lucide-react';

export interface NavItem {
    adminOnly?: boolean;
    key: string;
    label: string;
    path: string;
    icon: JSX.Element;
}

export const menuItems: NavItem[] = [
    {key: 'agents', label: '探针管理', path: '/admin/agents', icon: <Server size={17} strokeWidth={2}/>},
    {key: 'monitors', label: '服务监控', path: '/admin/monitors', icon: <Activity size={17} strokeWidth={2}/>},
    {key: 'ddns', label: 'DDNS', path: '/admin/ddns', icon: <Globe size={17} strokeWidth={2}/>},
    {key: 'comm-keys', label: '通信密钥', path: '/admin/api-keys', icon: <Key size={17} strokeWidth={2}/>},
    {key: 'api-keys', label: 'API 密钥', path: '/admin/manage-api-keys', icon: <Key size={17} strokeWidth={2}/>},
    {key: 'alert-records', label: '告警记录', path: '/admin/alert-records', icon: <AlertTriangle size={17} strokeWidth={2}/>},
    {key: 'accounts', label: '账号管理', path: '/admin/accounts', icon: <Users size={17} strokeWidth={2}/>, adminOnly: true},
    {key: 'settings', label: '系统设置', path: '/admin/settings', icon: <Settings size={17} strokeWidth={2}/>},
];

export const SIDEBAR_WIDTH = 248;
export const HEADER_HEIGHT = 60;
