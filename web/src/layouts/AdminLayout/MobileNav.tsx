import {useEffect, useState} from 'react';
import {useLocation, useNavigate} from 'react-router-dom';
import {ChevronRight, Menu, X} from 'lucide-react';
import {menuItems} from './menu';
import {cn} from '@/lib/utils';
import {useRuntimeConfig} from '@/api/runtime';

/** 小屏抽屉导航，与桌面侧栏保持一致的信息层级。 */
export const AdminMobileNav = ({isAdmin = false}: {isAdmin?: boolean}) => {
    const [open, setOpen] = useState(false);
    const location = useLocation();
    const navigate = useNavigate();
    const {data: runtime} = useRuntimeConfig();

    useEffect(() => {
        document.body.style.overflow = open ? 'hidden' : '';
        return () => { document.body.style.overflow = ''; };
    }, [open]);

    const goTo = (path: string) => {
        navigate(path);
        setOpen(false);
    };

    return (
        <>
            <button type="button" className="fixed top-2.5 left-3 z-[330] grid size-10 cursor-pointer place-items-center rounded-[11px] border border-[#dbe4ee] bg-white text-slate-700 dark:border-slate-700 dark:bg-[#171a21] dark:text-slate-200 lg:hidden" onClick={() => setOpen(true)} aria-label="打开导航">
                <Menu size={19}/>
            </button>
            {open && (
                <div className="fixed inset-0 z-[500]">
                    <button type="button" className="absolute inset-0 w-full cursor-pointer border-0 bg-slate-950/60 backdrop-blur-[3px]" onClick={() => setOpen(false)} aria-label="关闭导航遮罩"/>
                    <aside className="absolute inset-y-0 left-0 w-[min(86vw,320px)] overflow-y-auto bg-[#18202d] text-white shadow-[18px_0_50px_rgba(2,6,23,0.24)] dark:bg-[#111827]">
                        <div className="flex h-[60px] items-center gap-[11px] border-b border-white/[0.09] px-3.5">
                            <div className="grid size-9 shrink-0 place-items-center">
                                <img src="/api/logo" alt="Logo" className="size-[27px] object-contain" onError={(event) => { event.currentTarget.src = '/logo.png'; }}/>
                            </div>
                            <div className="min-w-0 leading-[1.2]">
                                <div className="overflow-hidden text-sm font-semibold tracking-[0.02em] text-ellipsis whitespace-nowrap">{runtime?.system.nameZh || 'Pika'}</div>
                                <div className="mt-1 overflow-hidden text-[10px] font-medium tracking-[0.11em] text-ellipsis whitespace-nowrap uppercase text-blue-100/60">管理控制台</div>
                            </div>
                            <button type="button" className="ml-auto grid size-9 cursor-pointer place-items-center rounded-[10px] border-0 bg-transparent text-slate-400 hover:bg-white/[0.08] hover:text-white" onClick={() => setOpen(false)} aria-label="关闭导航">
                                <X size={19}/>
                            </button>
                        </div>
                        <div className="px-[22px] pt-[18px] pb-2 text-[10px] font-semibold tracking-[0.13em] text-gray-200/45">管理</div>
                        <nav className="flex flex-col gap-1 px-3 pb-6" aria-label="主导航">
                            {menuItems.filter(item => !item.adminOnly || isAdmin).map((item) => {
                                const active = location.pathname === item.path || location.pathname.startsWith(`${item.path}/`);
                                return (
                                    <button
                                        key={item.key}
                                        type="button"
                                        onClick={() => goTo(item.path)}
                                        className={cn(
                                            'grid cursor-pointer grid-cols-[34px_minmax(0,1fr)_16px] items-center gap-2.5 rounded-[11px] border-0 px-2.5 py-[7px] text-left',
                                            active ? 'bg-[#1677ff]/20 text-[#f4f8ff]' : 'bg-transparent text-blue-50/90 hover:bg-white/[0.08]',
                                        )}
                                        aria-current={active ? 'page' : undefined}
                                    >
                                        <span className={cn('grid size-[34px] place-items-center bg-transparent', active ? 'text-[#69a7ff]' : 'text-blue-50/70')}>{item.icon}</span>
                                        <span className="min-w-0 text-[13px] font-semibold">
                                            <span>{item.label}</span>
                                        </span>
                                        <ChevronRight size={15}/>
                                    </button>
                                );
                            })}
                        </nav>
                    </aside>
                </div>
            )}
        </>
    );
};
