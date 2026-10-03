import {useLocation, useNavigate} from 'react-router-dom';
import {menuItems} from './menu';
import {cn} from '@/lib/utils';
import {useRuntimeConfig} from '@/api/runtime';

export const AdminSider = ({isAdmin = false}: {isAdmin?: boolean}) => {
    const location = useLocation();
    const navigate = useNavigate();
    const {data: runtime} = useRuntimeConfig();

    return (
        <aside className="fixed inset-y-0 left-0 z-[320] hidden w-[240px] overflow-hidden bg-[#18202d] text-white dark:bg-[#111827] lg:block">
            <div className="flex h-full flex-col">
                <div className="flex h-[60px] shrink-0 items-center gap-[11px] border-b border-white/[0.09] px-4">
                    <div className="grid size-9 shrink-0 place-items-center">
                        <img
                            src="/api/logo"
                            alt="Logo"
                            className="size-[27px] object-contain"
                            onError={(event) => { event.currentTarget.src = '/logo.png'; }}
                        />
                    </div>
                    <div className="min-w-0 leading-[1.2]">
                        <div className="overflow-hidden text-sm font-semibold tracking-[0.02em] text-ellipsis whitespace-nowrap text-white">{runtime?.system.nameZh || 'Pika'}</div>
                        <div className="mt-1 overflow-hidden text-[10px] font-medium tracking-[0.11em] text-ellipsis whitespace-nowrap uppercase text-blue-100/60">{runtime?.system.nameEn || 'Monitor'}</div>
                    </div>
                </div>
                <div className="px-[22px] pt-[18px] pb-2 text-[10px] font-semibold tracking-[0.13em] text-gray-200/45">管理</div>
                <nav className="thin-scrollbar flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto px-3 pb-5">
                    {menuItems.filter(item => !item.adminOnly || isAdmin).map((item) => {
                        const isActive = location.pathname === item.path || location.pathname.startsWith(`${item.path}/`);
                        return (
                            <button
                                key={item.key}
                                type="button"
                                onClick={() => navigate(item.path)}
                                className={cn(
                                    'flex min-h-[42px] cursor-pointer items-center gap-2.5 rounded-lg border-0 px-2.5 py-1.5 text-left text-[13px] transition-colors',
                                    isActive
                                        ? 'bg-[#1677ff]/20 text-[#f4f8ff] hover:bg-[#1677ff]/20'
                                        : 'bg-transparent text-gray-200/80 hover:bg-white/[0.06] hover:text-white',
                                )}
                                aria-current={isActive ? 'page' : undefined}
                            >
                                <span className={cn(
                                    'grid size-8 shrink-0 place-items-center bg-transparent',
                                    isActive ? 'text-[#69a7ff]' : 'text-gray-200/65',
                                )}>
                                    {item.icon}
                                </span>
                                <span className="overflow-hidden font-semibold text-ellipsis whitespace-nowrap">{item.label}</span>
                            </button>
                        );
                    })}
                </nav>

            </div>
        </aside>
    );
};
