import {type RefObject} from 'react';
import {useLocation, useNavigate} from 'react-router-dom';
import type {MenuProps} from 'antd';
import {App as AntApp, Avatar, Button, Dropdown, Tooltip} from 'antd';
import {BookOpen, ChevronRight, Eye, LogOut, Moon, Sun, User as UserIcon} from 'lucide-react';
import {logout} from '@/api/auth';
import {useRuntimeConfig} from '@/api/runtime';
import type {User} from '@/types';
import {menuItems} from './menu';

interface HeaderProps {
    userInfo: User | null;
    appliedTheme: 'light' | 'dark';
    themeButtonRef: RefObject<HTMLButtonElement | null>;
    onToggleTheme: () => void;
}

export const AdminHeader = ({userInfo, appliedTheme, themeButtonRef, onToggleTheme}: HeaderProps) => {
    const navigate = useNavigate();
    const location = useLocation();
    const {message: messageApi, modal} = AntApp.useApp();
    const {data: runtime} = useRuntimeConfig();
    const activeItem = menuItems.find((item) => location.pathname === item.path || location.pathname.startsWith(`${item.path}/`));
    const currentPageLabel = location.pathname.startsWith('/admin/agents-install/')
        ? '部署指南'
        : activeItem?.label || '管理后台';

    const handleLogout = () => {
        modal.confirm({
            title: '确认退出',
            content: '确定要退出登录吗？',
            onOk: async () => {
                try {
                    await logout();
                    localStorage.removeItem('token');
                    localStorage.removeItem('userInfo');
                    messageApi.success('已退出登录');
                    window.location.assign('/admin/login');
                } catch {
                    messageApi.error('退出登录失败，请重试');
                }
            },
        });
    };

    const userMenuItems: MenuProps['items'] = [
        {key: 'logout', icon: <LogOut size={16} strokeWidth={2}/>, label: '退出登录', onClick: handleLogout},
    ];

    return (
        <header className="fixed inset-x-0 top-0 z-[300] h-[60px] border-b border-[#e8ebf0] bg-white/95 backdrop-blur-xl dark:border-[#272b33] dark:bg-[#0f1115]/95 lg:left-[240px]">
            <div className="flex h-full items-center gap-4 px-4 lg:px-8">
                <div className="ml-[46px] flex min-w-0 items-center gap-2.5 lg:hidden">
                    <div className="grid size-[38px] shrink-0 place-items-center overflow-hidden">
                        <img
                            src="/api/logo"
                            alt="Logo"
                            className="size-[31px] object-contain"
                            onError={(e) => { e.currentTarget.src = '/logo.png'; }}
                        />
                    </div>
                    <div className="min-w-0 leading-[1.2]">
                        <p className="m-0 overflow-hidden text-sm font-bold text-ellipsis whitespace-nowrap text-[#1f2329] dark:text-[#e6e8ec]">{runtime?.system.nameZh || 'Pika'}</p>
                        <p className="mt-[3px] mb-0 text-[10px] text-slate-400 max-sm:hidden">管理控制台</p>
                    </div>
                </div>

                <div className="hidden min-w-0 items-center gap-[7px] text-xs font-medium text-slate-400 lg:flex">
                    <span>控制中心</span>
                    <ChevronRight size={13}/>
                    <span className="overflow-hidden text-ellipsis whitespace-nowrap text-[#4e5969] dark:text-[#b7bcc5]">{currentPageLabel}</span>
                </div>

                <div className="ml-auto flex items-center gap-1">
                    <div className="max-sm:hidden">
                        <Button
                            type="text"
                            icon={<Eye size={16} strokeWidth={2}/>}
                            onClick={() => window.open('/', '_blank')}
                            style={{color: appliedTheme === 'dark' ? '#cbd5e1' : '#646a73', fontSize: 12}}
                        >
                            公共页面
                        </Button>
                    </div>
                    <Button
                        type="text"
                        icon={<BookOpen size={16} strokeWidth={2}/>}
                        onClick={() => navigate('/admin/agents-install/one-click')}
                        style={{color: appliedTheme === 'dark' ? '#cbd5e1' : '#646a73', fontSize: 12}}
                    >
                        部署指南
                    </Button>

                    <Tooltip title={appliedTheme === 'dark' ? '切换到浅色模式' : '切换到暗黑模式'}>
                        <button
                            ref={themeButtonRef}
                            type="button"
                            onClick={onToggleTheme}
                            className="grid size-9 cursor-pointer place-items-center rounded-[9px] border-0 bg-transparent text-slate-500 hover:bg-slate-100 hover:text-slate-950 dark:text-slate-300 dark:hover:bg-slate-800 dark:hover:text-white"
                            aria-label={appliedTheme === 'dark' ? '切换到浅色模式' : '切换到暗黑模式'}
                        >
                            {appliedTheme === 'dark' ? <Sun size={17} strokeWidth={2}/> : <Moon size={17} strokeWidth={2}/>}
                        </button>
                    </Tooltip>

                    <Dropdown menu={{items: userMenuItems}} placement="bottomRight" trigger={['click']}>
                        <button
                            type="button"
                            className="flex min-w-0 cursor-pointer items-center gap-2 rounded-[10px] border-0 bg-transparent py-[3px] pr-2 pl-1 text-slate-700 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800"
                        >
                            <Avatar size={28} icon={<UserIcon size={15} strokeWidth={2}/>} style={{background: '#dbeafe', color: '#2563eb'}}/>
                            <span className="max-w-[120px] overflow-hidden text-xs font-semibold text-ellipsis whitespace-nowrap max-sm:hidden">{userInfo?.username || '访客'}</span>
                        </button>
                    </Dropdown>
                </div>
            </div>
        </header>
    );
};
