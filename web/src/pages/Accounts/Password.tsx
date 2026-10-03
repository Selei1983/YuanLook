import {useState} from 'react';
import {App, Button, Card, Form, Input} from 'antd';
import {changePassword} from '@/api/auth';
import {PasswordFields, errorMessage} from './fields';

export default function Password() {
    const {message} = App.useApp();
    const [loading, setLoading] = useState(false);
    const submit = async (values: {currentPassword: string; password: string}) => {
        setLoading(true);
        try {
            await changePassword({currentPassword: values.currentPassword, password: values.password});
            localStorage.removeItem('token');
            localStorage.removeItem('userInfo');
            message.success('密码已修改，请使用新密码登录');
            window.location.assign('/admin/login');
        } catch (error) { message.error(errorMessage(error)); }
        finally { setLoading(false); }
    };
    return <div className="max-w-xl">
        <h1 className="mt-0 text-xl font-semibold">修改密码</h1>
        <p className="mb-5 text-sm text-slate-500">修改后，所有设备上的旧登录会话都会失效。监控数据会保留。</p>
        <Card><Form layout="vertical" onFinish={submit} disabled={loading}>
            <Form.Item name="currentPassword" label="当前密码" rules={[{required: true, message: '请输入当前密码'}]}>
                <Input.Password autoComplete="current-password"/>
            </Form.Item>
            <PasswordFields/>
            <Button type="primary" htmlType="submit" loading={loading}>保存新密码</Button>
        </Form></Card>
    </div>;
}
