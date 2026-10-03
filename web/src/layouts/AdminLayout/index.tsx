import {useEffect, useMemo, useRef, useState} from 'react';
import {Outlet, useLocation, useNavigate} from 'react-router-dom';
import {ConfigProvider, theme, type ThemeConfig} from 'antd';
import {AdminHeader} from './Header';
import {AdminSider} from './Sider';
import {AdminMobileNav} from './MobileNav';
import {useThemeToggle} from './useThemeToggle';
import {HEADER_HEIGHT} from './menu';
import {getCurrentUser} from '@/api/auth';
import type {User} from '@/types';

const AdminLayout = () => {
    const navigate = useNavigate();
    const location = useLocation();
    // /admin/agents 前缀（含探针详情页）使用全宽布局
    const isWideCompactPage = location.pathname.startsWith('/admin/agents')
        || [
            '/admin/monitors',
            '/admin/ddns',
            '/admin/api-keys',
            '/admin/manage-api-keys',
            '/admin/alert-records',
            '/admin/agents-install/one-click',
            '/admin/agents-install/manual',
            '/admin/settings',
        ].includes(location.pathname);
    const [userInfo, setUserInfo] = useState<User | null>(null);
    const themeButtonRef = useRef<HTMLButtonElement>(null);
    const {appliedTheme, toggleTheme} = useThemeToggle(themeButtonRef);

    const themeConfig = useMemo<ThemeConfig>(() => ({
        algorithm: appliedTheme === 'dark' ? theme.darkAlgorithm : theme.defaultAlgorithm,
        token: {
            colorPrimary: '#1677ff',
            colorInfo: '#1677ff',
            colorBgLayout: appliedTheme === 'dark' ? '#0f1115' : '#f5f7fa',
            colorBgContainer: appliedTheme === 'dark' ? '#171a21' : '#ffffff',
            colorBgElevated: appliedTheme === 'dark' ? '#1c2028' : '#ffffff',
            colorBorder: appliedTheme === 'dark' ? '#30343d' : '#d9dee7',
            colorBorderSecondary: appliedTheme === 'dark' ? '#272b33' : '#e8ebf0',
            colorText: appliedTheme === 'dark' ? '#e6e8ec' : '#1f2329',
            colorTextSecondary: appliedTheme === 'dark' ? '#9ba1ab' : '#646a73',
            borderRadius: 8,
            borderRadiusLG: 10,
            controlHeight: 36,
            boxShadowSecondary: appliedTheme === 'dark'
                ? '0 16px 40px rgba(0, 0, 0, 0.28)'
                : '0 16px 40px rgba(15, 23, 42, 0.10)',
        },
        components: {
            Button: {primaryShadow: 'none', borderRadius: 8},
            Card: {
                headerBg: 'transparent',
                headerFontSize: 13,
                headerHeight: 46,
                headerPadding: 18,
                bodyPadding: 18,
            },
            Table: {
                headerBg: appliedTheme === 'dark' ? '#1c2028' : '#f7f8fa',
                headerColor: appliedTheme === 'dark' ? '#aeb4be' : '#4e5969',
                headerSplitColor: 'transparent',
                borderColor: appliedTheme === 'dark' ? '#272b33' : '#edf0f4',
                rowHoverBg: appliedTheme === 'dark' ? '#1b2028' : '#f7faff',
                fixedHeaderSortActiveBg: appliedTheme === 'dark' ? '#20242c' : '#f1f4f8',
                headerSortActiveBg: appliedTheme === 'dark' ? '#20242c' : '#f1f4f8',
                headerBorderRadius: 0,
            },
            Tabs: {horizontalItemGutter: 24},
        },
    }), [appliedTheme]);

    // 鉴权检查只在挂载时执行一次（不再依赖 location 导致每次路由切换都重查）
    useEffect(() => {
        const token = localStorage.getItem('token');
        const userInfoStr = localStorage.getItem('userInfo');
        if (!token || !userInfoStr) {
            navigate('/admin/login');
            return;
        }
        getCurrentUser().then(({data}) => { setUserInfo(data); localStorage.setItem('userInfo', JSON.stringify(data)); }).catch(() => navigate('/admin/login'));
    }, []);

    return (
        <ConfigProvider theme={themeConfig}>
            <div className="min-h-screen bg-[#f5f7fa] text-[#1f2329] dark:bg-[#0f1115] dark:text-[#e6e8ec]">
                <AdminHeader
                    userInfo={userInfo}
                    appliedTheme={appliedTheme}
                    themeButtonRef={themeButtonRef}
                    onToggleTheme={toggleTheme}
                />
                <AdminSider isAdmin={userInfo?.role === 'admin'}/>
                <div className="min-h-screen" style={{paddingTop: HEADER_HEIGHT}}>
                    <main
                        className={isWideCompactPage
                            ? 'px-4 pt-4 pb-[88px] lg:ml-[240px] lg:px-8 lg:pt-4 lg:pb-10'
                            : 'px-4 pt-[22px] pb-[88px] lg:ml-[240px] lg:px-8 lg:pt-6 lg:pb-10'}
                        style={{minHeight: `calc(100vh - ${HEADER_HEIGHT}px)`}}
                    >
                        <div className={isWideCompactPage ? 'w-full' : 'mx-auto w-full max-w-[1320px]'}>
                            <Outlet/>
                        </div>
                    </main>
                </div>
                <AdminMobileNav isAdmin={userInfo?.role === 'admin'}/>
            </div>
        </ConfigProvider>
    );
};

export default AdminLayout;
