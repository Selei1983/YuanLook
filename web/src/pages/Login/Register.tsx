import {useEffect, useState} from 'react';
import {Link} from 'react-router-dom';
import {Alert, App, Button, Card, Form, Result} from 'antd';
import {getAuthConfig, register} from '@/api/auth';
import {UsernameField, PasswordFields, errorMessage} from '@/pages/Accounts/fields';

export default function Register() {
    const {message} = App.useApp();
    const [loading, setLoading] = useState(false);
    const [enabled, setEnabled] = useState<boolean>();
    const [registered, setRegistered] = useState(false);
    const [configError, setConfigError] = useState(false);
    useEffect(() => { getAuthConfig().then(({data}) => setEnabled(data.registrationEnabled)).catch(() => setConfigError(true)); }, []);
    const submit = async (values: {username: string; password: string}) => {
        setLoading(true);
        try {
            await register({username: values.username, password: values.password});
            setRegistered(true);
        } catch (error) { message.error(errorMessage(error)); }
        finally { setLoading(false); }
    };
    return (
        <div className="flex min-h-screen items-center justify-center bg-slate-50 p-5 dark:bg-slate-950">
            <div className="w-full max-w-md">
                <Card>
                    <h1 className="mt-0 text-2xl font-semibold">注册 YuanLook</h1>
                    <p className="mb-6 text-sm text-slate-500">创建账号后即可使用独立的监控空间。</p>
                    {registered ? <Result status="success" title="注册成功" subTitle="账号已启用，现在可以登录并添加自己的服务器。" extra={<Link to="/admin/login"><Button type="primary">前往登录</Button></Link>}/> : enabled ? (
                        <Form layout="vertical" onFinish={submit} requiredMark={false} disabled={loading}>
                            <UsernameField/>
                            <PasswordFields/>
                            <Button type="primary" htmlType="submit" block loading={loading}>注册账号</Button>
                        </Form>
                    ) : <Alert type={configError ? 'error' : 'info'} title={configError ? '无法获取注册配置，请刷新重试' : enabled === false ? '注册暂未开放，请联系管理员创建账号' : '正在加载…'}/>}
                    {!registered && <p className="mb-0 mt-5 text-center text-sm"><Link to="/admin/login">已有账号？返回登录</Link></p>}
                </Card>
            </div>
        </div>
    );
}
