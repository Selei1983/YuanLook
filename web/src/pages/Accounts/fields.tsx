import {Form, Input} from 'antd';

export const UsernameField = () => (
    <Form.Item name="username" label="用户名" rules={[{required: true, message: '请输入用户名'}, {pattern: /^[a-zA-Z0-9][a-zA-Z0-9_-]{2,31}$/, message: '3–32 位字母、数字、下划线或短横线，以字母或数字开头'}]}>
        <Input autoComplete="username" maxLength={32} placeholder="例如 yuanlook"/>
    </Form.Item>
);

export const PasswordFields = () => (
    <>
        <Form.Item name="password" label="新密码" rules={[{required: true, message: '请输入密码'}, {validator: (_, value: string) => {
            const bytes = new TextEncoder().encode(value || '').length;
            return bytes >= 10 && bytes <= 72 ? Promise.resolve() : Promise.reject(new Error('密码长度须为 10–72 字节'));
        }}]} extra="至少 10 位；中文等字符会占用多个字节。">
            <Input.Password autoComplete="new-password" maxLength={72}/>
        </Form.Item>
        <Form.Item name="confirmPassword" label="确认密码" dependencies={['password']} rules={[{required: true, message: '请再次输入密码'}, ({getFieldValue}) => ({validator: (_, value) => value === getFieldValue('password') ? Promise.resolve() : Promise.reject(new Error('两次输入的密码不一致'))})]}>
            <Input.Password autoComplete="new-password" maxLength={72}/>
        </Form.Item>
    </>
);

export const errorMessage = (error: unknown) => error instanceof Error ? error.message : '操作失败，请重试';
