import {useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {Alert, App, Button, Card, Form, Input, Modal, Result, Space, Table, Tag} from 'antd';
import {getAccounts, getCurrentUser, createAccount, updateAccount, type Account} from '@/api/auth';
import {UsernameField, PasswordFields, errorMessage} from './fields';

export default function Accounts() {
    const {message, modal} = App.useApp();
    const client = useQueryClient();
    const current = useQuery({queryKey: ['current-user'], queryFn: async () => (await getCurrentUser()).data});
    const accounts = useQuery({queryKey: ['platform-accounts'], queryFn: async () => (await getAccounts()).data, enabled: current.data?.role === 'admin'});
    const [search, setSearch] = useState('');
    const [editor, setEditor] = useState<{username?: string} | null>(null);
    const [saving, setSaving] = useState(false);
    const openEditor = (username?: string) => setEditor({username});
    const save = async (values: {username: string; password: string}) => {
        setSaving(true);
        try {
            if (editor?.username) await updateAccount(editor.username, {password: values.password});
            else await createAccount({username: values.username, password: values.password});
            setEditor(null);
            message.success(editor?.username ? '密码已重置，旧登录会话已失效' : '账号已创建并启用');
            await client.invalidateQueries({queryKey: ['platform-accounts']});
        } catch (error) { message.error(errorMessage(error)); }
        finally { setSaving(false); }
    };
    const toggle = (account: Account) => modal.confirm({
        title: `${account.enabled ? '停用' : '启用'}账号 ${account.username}？`,
        content: account.enabled ? '停用后该账号将无法登录或调用 API。已有监控仍继续运行，数据不会删除。' : '启用后，用户可以重新登录原有监控空间。',
        okText: account.enabled ? '停用账号' : '启用账号', cancelText: '取消',
        okButtonProps: {danger: account.enabled},
        onOk: async () => {
            try {
                await updateAccount(account.username, {enabled: !account.enabled});
                message.success('账号状态已更新');
                await client.invalidateQueries({queryKey: ['platform-accounts']});
            } catch (error) { message.error(errorMessage(error)); throw error; }
        },
    });
    if (current.isPending) return <p>正在加载…</p>;
    if (current.isError) return <Alert type="error" title={errorMessage(current.error)} action={<Button onClick={() => current.refetch()}>重试</Button>}/>;
    if (current.data?.role !== 'admin') return <Result status="403" title="仅平台管理员可以管理账号" subTitle="你的监控空间和数据独立保存。"/>;
    return <>
        <div className="mb-5 flex flex-wrap items-start justify-between gap-4">
            <div><h1 className="m-0 text-xl font-semibold">账号管理</h1><p className="mb-0 mt-2 text-sm text-slate-500">管理平台登录账号；每个账号拥有独立的监控空间。</p></div>
            <Button type="primary" onClick={() => openEditor()}>创建账号</Button>
        </div>
        <Card>
            <div className="mb-4 max-w-xs"><Input.Search placeholder="搜索用户名" allowClear value={search} onChange={e => setSearch(e.target.value)}/></div>
            {accounts.isError && <Alert type="error" title={errorMessage(accounts.error)} action={<Button onClick={() => accounts.refetch()}>重试</Button>}/>}
            <Table<Account> rowKey="username" loading={accounts.isPending} scroll={{x: 700}} dataSource={(accounts.data || []).filter(a => a.username.toLowerCase().includes(search.toLowerCase()))} pagination={{pageSize: 20, hideOnSinglePage: true}} columns={[
                {title: '用户名', dataIndex: 'username'},
                {title: '角色', dataIndex: 'role', render: value => value === 'admin' ? '平台管理员' : '普通用户'},
                {title: '状态', dataIndex: 'enabled', render: value => <Tag color={value ? 'green' : 'default'}>{value ? '已启用' : '已停用'}</Tag>},
                {title: '创建时间', dataIndex: 'createdAt', render: value => new Date(value).toLocaleString()},
                {title: '操作', render: (_, account) => account.role === 'admin' ? <span className="text-xs text-slate-400">在个人菜单修改密码</span> : <Space>
                    <Button size="small" onClick={() => openEditor(account.username)}>重置密码</Button>
                    <Button size="small" danger={account.enabled} onClick={() => toggle(account)}>{account.enabled ? '停用' : '启用'}</Button>
                </Space>},
            ]}/>
        </Card>
        <Modal title={editor?.username ? `重置 ${editor.username} 的密码` : '创建账号'} open={editor !== null} onCancel={() => !saving && setEditor(null)} footer={null} destroyOnHidden mask={{closable: !saving}}>
            <Form key={editor?.username ?? 'new'} layout="vertical" onFinish={save} preserve={false} disabled={saving}>
                {!editor?.username && <UsernameField/>}
                <PasswordFields/>
                <Space><Button onClick={() => setEditor(null)}>取消</Button><Button type="primary" htmlType="submit" loading={saving}>{editor?.username ? '重置密码' : '创建并启用'}</Button></Space>
            </Form>
        </Modal>
    </>;
}
