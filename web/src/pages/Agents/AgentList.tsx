import {createContext, useContext, useMemo, useState} from 'react';
import type {CSSProperties, HTMLAttributes} from 'react';
import {Link, useNavigate} from 'react-router-dom';
import type {MenuProps} from 'antd';
import {App, Button, Dropdown, Form, Input, Select, Space, Tag} from 'antd';
import type {ColumnsType} from 'antd/es/table';
import {Activity, Edit, Eye, EyeOff, FileWarning, GripVertical, Lock, MoreVertical, Play, Plus, PowerOff, RefreshCw, Shield, Tags, Trash2} from 'lucide-react';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import type {DragEndEvent} from '@dnd-kit/core';
import {DndContext, KeyboardSensor, PointerSensor, useSensor, useSensors} from '@dnd-kit/core';
import {restrictToVerticalAxis} from '@dnd-kit/modifiers';
import {
    arrayMove,
    SortableContext,
    sortableKeyboardCoordinates,
    useSortable,
    verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import {CSS} from '@dnd-kit/utilities';
import dayjs from 'dayjs';
import {deleteAgent, getTags, listAgentsByAdmin, updateAgentEnabled, updateAgentOrder} from '@/api/agent.ts';
import type {Agent} from '@/types';
import {getErrorMessage} from '@/lib/utils';
import {PageHeader} from '@/components/PageHeader';
import {AdminDataTable} from '@/components/AdminDataTable';
import AgentEditModal from './AgentEditModal';
import BatchTagsModal from './BatchTagsModal';
import BatchTamperProtectionModal from './BatchTamperProtectionModal';
import BatchSSHLoginConfigModal from './BatchSSHLoginConfigModal';
import BatchVisibilityModal from './BatchVisibilityModal';
import BatchTrafficStatsModal from './BatchTrafficStatsModal';

type SortableRowHook = ReturnType<typeof useSortable>;

interface SortableRowContextValue {
    listeners?: SortableRowHook['listeners'];
    setActivatorNodeRef?: SortableRowHook['setActivatorNodeRef'];
}

interface SortableRowProps extends HTMLAttributes<HTMLTableRowElement> {
    'data-row-key': string;
}

const SortableRowContext = createContext<SortableRowContextValue>({});
const SortingDisabledContext = createContext(false);

const DragHandle = () => {
    const disabled = useContext(SortingDisabledContext);
    const {listeners, setActivatorNodeRef} = useContext(SortableRowContext);

    return (
        <Button
            ref={setActivatorNodeRef}
            type="text"
            size="small"
            icon={<GripVertical size={16}/>}
            disabled={disabled}
            aria-label="拖动调整探针顺序"
            title="拖动调整顺序"
            style={{cursor: disabled ? 'not-allowed' : 'grab', touchAction: 'none'}}
            {...listeners}
        />
    );
};

const SortableRow = (props: SortableRowProps) => {
    const disabled = useContext(SortingDisabledContext);
    const {
        attributes,
        listeners,
        setNodeRef,
        setActivatorNodeRef,
        transform,
        transition,
        isDragging,
    } = useSortable({id: props['data-row-key'], disabled});
    const style: CSSProperties = {
        ...props.style,
        transform: CSS.Translate.toString(transform),
        transition,
        ...(isDragging ? {position: 'relative', zIndex: 2} : {}),
    };
    const contextValue = useMemo(
        () => ({listeners, setActivatorNodeRef}),
        [listeners, setActivatorNodeRef],
    );

    return (
        <SortableRowContext.Provider value={contextValue}>
            <tr {...props} ref={setNodeRef} style={style} {...attributes}/>
        </SortableRowContext.Provider>
    );
};

const AgentList = () => {
    const navigate = useNavigate();
    const {message: messageApi, modal} = App.useApp();
    const queryClient = useQueryClient();

    const [searchForm] = Form.useForm();
    const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
    const [editModalVisible, setEditModalVisible] = useState(false);
    const [batchTagModalVisible, setBatchTagModalVisible] = useState(false);
    const [batchTamperModalVisible, setBatchTamperModalVisible] = useState(false);
    const [batchSSHModalVisible, setBatchSSHModalVisible] = useState(false);
    const [batchVisibilityModalVisible, setBatchVisibilityModalVisible] = useState(false);
    const [batchTrafficModalVisible, setBatchTrafficModalVisible] = useState(false);
    const [editingAgentId, setEditingAgentId] = useState<string | undefined>(undefined);

    // 过滤条件
    const [keyword, setKeyword] = useState('');
    const [status, setStatus] = useState<string | undefined>(undefined);
    const [selectedTags, setSelectedTags] = useState<string[]>([]);
    const sensors = useSensors(
        useSensor(PointerSensor, {activationConstraint: {distance: 4}}),
        useSensor(KeyboardSensor, {coordinateGetter: sortableKeyboardCoordinates}),
    );

    const {data: tags = []} = useQuery({
        queryKey: ['admin', 'agents', 'tags'],
        queryFn: async () => {
            const response = await getTags();
            return response.data.tags || [];
        },
    });

    const {
        data: agents = [],
        isLoading,
        isFetching,
        refetch,
    } = useQuery({
        queryKey: ['admin', 'agents'],
        queryFn: async () => {
            const response = await listAgentsByAdmin();
            return response.data;
        },
    });

    const deleteMutation = useMutation({
        mutationFn: (agentId: string) => deleteAgent(agentId),
        onSuccess: () => {
            messageApi.success('探针删除成功');
            queryClient.invalidateQueries({queryKey: ['admin', 'agents']});
        },
        onError: (error: unknown) => {
            messageApi.error(getErrorMessage(error, '删除探针失败'));
        },
    });

    const enabledMutation = useMutation({
        mutationFn: ({agentId, enabled}: {agentId: string; enabled: boolean}) => updateAgentEnabled(agentId, enabled),
        onSuccess: (_response, variables) => {
            messageApi.success(variables.enabled ? '主机已启用' : '主机已禁用');
            queryClient.invalidateQueries({queryKey: ['admin', 'agents']});
        },
        onError: (error: unknown, variables) => {
            messageApi.error(getErrorMessage(error, variables.enabled ? '启用主机失败' : '禁用主机失败'));
        },
    });

    const orderMutation = useMutation({
        mutationFn: (agentIds: string[]) => updateAgentOrder(agentIds),
        onMutate: async (agentIds) => {
            await queryClient.cancelQueries({queryKey: ['admin', 'agents'], exact: true});
            const previousAgents = queryClient.getQueryData<Agent[]>(['admin', 'agents']);
            if (previousAgents) {
                const agentsByID = new Map(previousAgents.map(agent => [agent.id, agent]));
                const reorderedAgents = agentIds
                    .map(agentID => agentsByID.get(agentID))
                    .filter((agent): agent is Agent => agent !== undefined);
                if (reorderedAgents.length === previousAgents.length) {
                    queryClient.setQueryData(['admin', 'agents'], reorderedAgents);
                }
            }
            return {previousAgents};
        },
        onSuccess: () => {
            messageApi.success('探针排序已保存');
        },
        onError: (error: unknown, _agentIds, context) => {
            if (context?.previousAgents) {
                queryClient.setQueryData(['admin', 'agents'], context.previousAgents);
            }
            messageApi.error(getErrorMessage(error, '保存探针排序失败'));
        },
        onSettled: () => {
            queryClient.invalidateQueries({queryKey: ['admin', 'agents'], exact: true});
        },
    });

    const handleSearch = () => {
        const values = searchForm.getFieldsValue();
        setKeyword(values.keyword?.trim() || '');
        setStatus(values.status);
    };

    const handleReset = () => {
        searchForm.resetFields();
        setKeyword('');
        setStatus(undefined);
        setSelectedTags([]);
    };

    const handleEdit = (agent: Agent) => {
        setEditingAgentId(agent.id);
        setEditModalVisible(true);
    };

    const handleDelete = (agent: Agent) => {
        modal.confirm({
            title: '删除探针',
            content: (
                <div>
                    <p>确定要删除探针「{agent.name || agent.hostname}」吗？</p>
                    <p className="text-red-500 text-sm mt-2">
                        警告：此操作将删除探针及其所有相关数据（指标数据、监控统计、审计结果等），且不可恢复！
                    </p>
                </div>
            ),
            okText: '确认删除',
            cancelText: '取消',
            okButtonProps: {danger: true},
            centered: true,
            onOk: async () => {
                try {
                    await deleteMutation.mutateAsync(agent.id);
                } catch {
                    // 错误提示已在 mutation 中处理
                }
            },
        });
    };

    const handleEnabledChange = (agent: Agent) => {
        if (!agent.enabled) {
            enabledMutation.mutate({agentId: agent.id, enabled: true});
            return;
        }

        modal.confirm({
            title: '禁用主机',
            content: `确定要禁用主机「${agent.name || agent.hostname}」吗？禁用期间接收到的数据将被忽略，也不会触发告警。`,
            okText: '确认禁用',
            cancelText: '取消',
            okButtonProps: {danger: true},
            centered: true,
            onOk: () => enabledMutation.mutateAsync({agentId: agent.id, enabled: false}),
        });
    };

    const handleBatchTags = () => {
        if (selectedRowKeys.length === 0) {
            messageApi.warning('请先选择要操作的探针');
            return;
        }
        setBatchTagModalVisible(true);
    };

    const handleBatchTamperConfig = () => {
        if (selectedRowKeys.length === 0) {
            messageApi.warning('请先选择要操作的探针');
            return;
        }
        setBatchTamperModalVisible(true);
    };

    const handleBatchSSHConfig = () => {
        if (selectedRowKeys.length === 0) {
            messageApi.warning('请先选择要操作的探针');
            return;
        }
        setBatchSSHModalVisible(true);
    };

    const handleBatchVisibility = () => {
        if (selectedRowKeys.length === 0) {
            messageApi.warning('请先选择要操作的探针');
            return;
        }
        setBatchVisibilityModalVisible(true);
    };

    const handleBatchTrafficConfig = () => {
        if (selectedRowKeys.length === 0) {
            messageApi.warning('请先选择要操作的探针');
            return;
        }
        setBatchTrafficModalVisible(true);
    };

    // 前端过滤数据
    const filteredAgents = useMemo(() => {
        let result = agents || [];

        // 关键字过滤
        if (keyword) {
            const lowerKeyword = keyword.toLowerCase();
            result = result.filter((agent: Agent) => {
                return (
                    agent.name?.toLowerCase().includes(lowerKeyword) ||
                    agent.hostname?.toLowerCase().includes(lowerKeyword) ||
                    agent.ip?.toLowerCase().includes(lowerKeyword) ||
                    agent.ipv4?.toLowerCase().includes(lowerKeyword) ||
                    agent.ipv6?.toLowerCase().includes(lowerKeyword)
                );
            });
        }

        // 状态过滤
        if (status === 'disabled') {
            result = result.filter((agent: Agent) => !agent.enabled);
        } else if (status) {
            const statusValue = status === 'online' ? 1 : 0;
            result = result.filter((agent: Agent) => agent.enabled && agent.status === statusValue);
        }

        // 标签过滤
        if (selectedTags.length > 0) {
            result = result.filter((agent: Agent) => {
                return selectedTags.some(tag => agent.tags?.includes(tag));
            });
        }

        return result;
    }, [agents, keyword, status, selectedTags]);

    const handleDragEnd = ({active, over}: DragEndEvent) => {
        if (!over || active.id === over.id || orderMutation.isPending) {
            return;
        }

        const activeIndex = filteredAgents.findIndex(agent => agent.id === active.id);
        const overIndex = filteredAgents.findIndex(agent => agent.id === over.id);
        if (activeIndex < 0 || overIndex < 0) {
            return;
        }

        const reorderedVisibleAgents = arrayMove(filteredAgents, activeIndex, overIndex);
        const visibleAgentIDs = new Set(filteredAgents.map(agent => agent.id));
        let visibleIndex = 0;
        const reorderedAgents = agents.map(agent => {
            if (!visibleAgentIDs.has(agent.id)) {
                return agent;
            }
            const reorderedAgent = reorderedVisibleAgents[visibleIndex];
            visibleIndex += 1;
            return reorderedAgent;
        });
        orderMutation.mutate(reorderedAgents.map(agent => agent.id));
    };

    const columns: ColumnsType<Agent> = [
        {
            title: '排序',
            key: 'sort',
            fixed: 'left',
            align: 'center',
            width: 56,
            render: () => <DragHandle/>,
        },
        {
            title: '名称',
            dataIndex: 'name',
            key: 'name',
            fixed: 'left',
            width: 220,
            render: (_, record) => (
                <div className="space-y-1">
                    <div className="font-medium">
                        <Link to={`/admin/agents/${record.id}`}>
                            {record.name || record.hostname}
                        </Link>
                    </div>
                    <Tag color="geekblue" variant={'filled'}>{record.os} · {record.arch}</Tag>
                </div>
            ),
        },
        {
            title: '标签',
            dataIndex: 'tags',
            key: 'tags',
            width: 200,
            render: (_, record) => (
                <div className={'flex items-center gap-1'}>
                    {record.tags?.map((tag, index) => (
                        <Tag key={index} color="blue" variant={'filled'}>
                            {tag}
                        </Tag>
                    ))}
                </div>
            ),
        },
        {
            title: '状态',
            dataIndex: 'status',
            key: 'status',
            width: 80,
            render: (_, record) => {
                if (!record.enabled) {
                    return <Tag color="warning">已禁用</Tag>;
                }
                return (
                    <Tag color={record.status === 1 ? 'success' : 'default'}>
                        {record.status === 1 ? '在线' : '离线'}
                    </Tag>
                );
            },
        },
        {
            title: '可见性',
            dataIndex: 'visibility',
            key: 'visibility',
            width: 100,
            render: () => (
                <Tag color="orange">
                    登录可见
                </Tag>
            ),
        },
        {
            title: '主机名',
            dataIndex: 'hostname',
            key: 'hostname',
            ellipsis: true,
            width: 150,
        },
        {
            title: '通信地址',
            dataIndex: 'ip',
            key: 'ip',
            ellipsis: true,
            width: 160,
            render: (value) => (
                <span className="font-mono text-xs">{value || '-'}</span>
            ),
        },
        {
            title: 'IPv4',
            dataIndex: 'ipv4',
            key: 'ipv4',
            ellipsis: true,
            width: 160,
            render: (value) => (
                <span className="font-mono text-xs">{value || '-'}</span>
            ),
        },
        {
            title: 'IPv6',
            dataIndex: 'ipv6',
            key: 'ipv6',
            ellipsis: true,
            width: 200,
            render: (value) => (
                <span className="font-mono text-xs">{value || '-'}</span>
            ),
        },
        {
            title: '版本',
            dataIndex: 'version',
            key: 'version',
            width: 120,
            render: (value) => (
                <span className="font-mono text-xs whitespace-nowrap">{value || '-'}</span>
            ),
        },
        {
            title: '到期时间',
            dataIndex: 'expireTime',
            key: 'expireTime',
            width: 100,
            render: (val) => {
                if (!val) return '-';
                const expireDate = new Date(val as number);
                const now = new Date();
                const isExpired = expireDate < now;
                const daysLeft = Math.ceil((expireDate.getTime() - now.getTime()) / (1000 * 60 * 60 * 24));

                return (
                    <div className="space-y-1">
                        <div>{expireDate.toLocaleDateString('zh-CN')}</div>
                        {isExpired ? (
                            <Tag color="red" variant={'filled'}>已过期</Tag>
                        ) : daysLeft <= 7 ? (
                            <Tag color="orange" variant={'filled'}>{daysLeft}天后到期</Tag>
                        ) : daysLeft <= 30 ? (
                            <Tag color="gold" variant={'filled'}>{daysLeft}天后到期</Tag>
                        ) : null}
                    </div>
                );
            },
        },
        {
            title: '流量统计',
            key: 'trafficStats',
            width: 120,
            render: (_, record) => {
                const trafficStats = record.trafficStats;
                if (!trafficStats || !trafficStats.enabled) {
                    return <Tag variant={'filled'}>未启用</Tag>;
                }
                return (
                    <div className="space-y-1">
                        <Tag color="green" variant={'filled'}>已启用</Tag>
                        {trafficStats.limit > 0 && (
                            <span className="text-xs text-gray-500">
                                {(trafficStats.used / (1024 * 1024 * 1024)).toFixed(2)}GB / {(trafficStats.limit / (1024 * 1024 * 1024)).toFixed(0)}GB
                            </span>
                        )}
                    </div>
                );
            },
        },
        {
            title: '防篡改保护',
            key: 'tamperProtect',
            width: 120,
            render: (_, record) => {
                const config = record.tamperProtectConfig;
                if (!config || !config.enabled) {
                    return <Tag variant={'filled'}>未启用</Tag>;
                }
                return (
                    <div className="space-y-1">
                        <Tag color="green" variant={'filled'}>已启用</Tag>
                        {config.paths && config.paths.length > 0 && (
                            <span className="text-xs text-gray-500">{config.paths.length} 个路径</span>
                        )}
                    </div>
                );
            },
        },
        {
            title: 'SSH登录监控',
            key: 'sshLogin',
            width: 120,
            render: (_, record) => {
                const config = record.sshLoginConfig;
                if (!config || !config.enabled) {
                    return <Tag variant={'filled'}>未启用</Tag>;
                }
                return <Tag color="green" variant={'filled'}>已启用</Tag>;
            },
        },
        {
            title: '备注',
            dataIndex: 'remark',
            key: 'remark',
            width: 200,
            ellipsis: true,
            render: (value) => (
                <span className="text-gray-700">{value || '-'}</span>
            ),
        },
        {
            title: '最后活跃时间',
            dataIndex: 'lastSeenAt',
            key: 'lastSeenAt',
            width: 180,
            render: (value) => (value ? dayjs(value).format('YYYY-MM-DD HH:mm') : '-'),
        },
        {
            title: '操作',
            key: 'action',
            width: 150,
            fixed: 'right',
            render: (_, record) => {
                const menuItems: MenuProps['items'] = [
                    {
                        key: 'view',
                        label: '查看详情',
                        icon: <Eye size={14}/>,
                        onClick: () => navigate(`/admin/agents/${record.id}`),
                    },
                    {
                        key: 'audit',
                        label: '安全审计',
                        icon: <Shield size={14}/>,
                        onClick: () => navigate(`/admin/agents/${record.id}?tab=audit`),
                    },
                    {
                        key: 'tamper',
                        label: '防篡改保护',
                        icon: <FileWarning size={14}/>,
                        onClick: () => navigate(`/admin/agents/${record.id}?tab=tamper`),
                    },
                    {
                        key: 'ssh-login',
                        label: 'SSH 登录监控',
                        icon: <Lock size={14}/>,
                        onClick: () => navigate(`/admin/agents/${record.id}?tab=ssh-login`),
                    },
                    {
                        key: 'edit',
                        label: '编辑信息',
                        icon: <Edit size={14}/>,
                        onClick: () => handleEdit(record),
                    },
                    {
                        type: 'divider',
                    },
                    {
                        key: 'enabled',
                        label: record.enabled ? '禁用主机' : '启用主机',
                        icon: record.enabled ? <PowerOff size={14}/> : <Play size={14}/>,
                        danger: record.enabled,
                        disabled: enabledMutation.isPending,
                        onClick: () => handleEnabledChange(record),
                    },
                    {
                        type: 'divider',
                    },
                    {
                        key: 'delete',
                        label: '删除探针',
                        icon: <Trash2 size={14}/>,
                        danger: true,
                        onClick: () => handleDelete(record),
                    },
                ];

                return (
                    <Space size="small">
                        <Button
                            type="link"
                            icon={<Eye size={14}/>}
                            onClick={() => navigate(`/admin/agents/${record.id}`)}
                            style={{padding: 0}}
                        >
                            详情
                        </Button>
                        <Dropdown menu={{items: menuItems}} trigger={['click']} placement="bottomRight">
                            <Button
                                type="link"
                                icon={<MoreVertical size={14}/>}
                                style={{padding: 0}}
                            />
                        </Dropdown>
                    </Space>
                );
            },
        },
    ];

    const batchMenuItems: MenuProps['items'] = [
        {key: 'tags', icon: <Tags size={15}/>, label: '修改标签', onClick: handleBatchTags},
        {key: 'visibility', icon: <EyeOff size={15}/>, label: '修改可见性', onClick: handleBatchVisibility},
        {key: 'traffic', icon: <Activity size={15}/>, label: '配置流量统计', onClick: handleBatchTrafficConfig},
        {key: 'tamper', icon: <FileWarning size={15}/>, label: '配置防篡改保护', onClick: handleBatchTamperConfig},
        {key: 'ssh', icon: <Lock size={15}/>, label: '配置 SSH 登录监控', onClick: handleBatchSSHConfig},
    ];

    return (
        <div className="space-y-4">
            <PageHeader
                title="探针管理"
                extra={(
                    <>
                        <Form
                            form={searchForm}
                            layout="inline"
                            onFinish={handleSearch}
                            className="min-w-0 max-sm:w-full"
                            style={{display: 'flex', flexWrap: 'wrap', justifyContent: 'flex-end', gap: 8}}
                        >
                            <Form.Item name="keyword" style={{margin: 0}}>
                                <Input
                                    allowClear
                                    placeholder="搜索名称、主机名或 IP"
                                    style={{width: 220}}
                                />
                            </Form.Item>
                            <Form.Item name="status" style={{margin: 0}}>
                                <Select
                                    placeholder="全部状态"
                                    allowClear
                                    style={{width: 112}}
                                    options={[
                                        {label: '在线', value: 'online'},
                                        {label: '离线', value: 'offline'},
                                        {label: '已禁用', value: 'disabled'},
                                    ]}
                                />
                            </Form.Item>
                            <Form.Item style={{margin: 0}}>
                                <Space size={8}>
                                    <Button type="primary" htmlType="submit">查询</Button>
                                    <Button onClick={handleReset}>重置</Button>
                                </Space>
                            </Form.Item>
                        </Form>
                        <Dropdown menu={{items: batchMenuItems}} trigger={['click']} disabled={selectedRowKeys.length === 0}>
                            <Button icon={<Tags size={16}/>} disabled={selectedRowKeys.length === 0}>
                                批量操作{selectedRowKeys.length > 0 ? ` (${selectedRowKeys.length})` : ''}
                            </Button>
                        </Dropdown>
                    </>
                )}
                actions={[
                    {
                        key: 'register',
                        label: '注册探针',
                        icon: <Plus size={16}/>,
                        onClick: () => navigate('/admin/agents-install/one-click'),
                        type: 'primary',
                    },
                    {
                        key: 'refresh',
                        label: '刷新',
                        icon: <RefreshCw size={16}/>,
                        onClick: () => refetch(),
                    },
                ]}
            />

            {tags.length > 0 && (
                <div className="flex min-h-7 flex-wrap items-center gap-1.5">
                    <span className="text-xs text-[#646a73] dark:text-[#9ba1ab]">标签：</span>
                    <Tag.CheckableTag
                        checked={selectedTags.length === 0}
                        onChange={() => setSelectedTags([])}
                        style={{borderRadius: 6, margin: 0}}
                    >
                        全部
                    </Tag.CheckableTag>
                    {tags.map((tag) => (
                        <Tag.CheckableTag
                            key={tag}
                            checked={selectedTags.includes(tag)}
                            onChange={(checked) => {
                                if (checked) {
                                    setSelectedTags([...selectedTags, tag]);
                                } else {
                                    setSelectedTags(selectedTags.filter(t => t !== tag));
                                }
                            }}
                            style={{borderRadius: 6, margin: 0}}
                        >
                            {tag}
                        </Tag.CheckableTag>
                    ))}
                </div>
            )}

            <SortingDisabledContext.Provider
                value={isLoading || isFetching || orderMutation.isPending || filteredAgents.length < 2}
            >
                <DndContext
                    sensors={sensors}
                    modifiers={[restrictToVerticalAxis]}
                    onDragEnd={handleDragEnd}
                >
                    <SortableContext
                        items={filteredAgents.map(agent => agent.id)}
                        strategy={verticalListSortingStrategy}
                    >
                        <AdminDataTable<Agent>
                            columns={columns}
                            dataSource={filteredAgents}
                            loading={isLoading || isFetching}
                            rowKey="id"
                            components={{body: {row: SortableRow}}}
                            scroll={{x: 2600}}
                            tableLayout="fixed"
                            rowSelection={{
                                selectedRowKeys,
                                onChange: (keys) => setSelectedRowKeys(keys),
                                preserveSelectedRowKeys: true,
                            }}
                            pagination={false}
                        />
                    </SortableContext>
                </DndContext>
            </SortingDisabledContext.Provider>

            <AgentEditModal
                open={editModalVisible}
                agentId={editingAgentId}
                existingTags={tags}
                onCancel={() => {
                    setEditModalVisible(false);
                    setEditingAgentId(undefined);
                }}
                onSuccess={() => {
                    setEditModalVisible(false);
                    setEditingAgentId(undefined);
                }}
            />

            <BatchTagsModal
                open={batchTagModalVisible}
                agentIds={selectedRowKeys as string[]}
                existingTags={tags}
                onCancel={() => setBatchTagModalVisible(false)}
                onSuccess={() => {
                    setBatchTagModalVisible(false);
                    setSelectedRowKeys([]);
                }}
            />

            <BatchTamperProtectionModal
                open={batchTamperModalVisible}
                agentIds={selectedRowKeys as string[]}
                onCancel={() => setBatchTamperModalVisible(false)}
                onSuccess={() => {
                    setBatchTamperModalVisible(false);
                    setSelectedRowKeys([]);
                }}
            />

            <BatchSSHLoginConfigModal
                open={batchSSHModalVisible}
                agentIds={selectedRowKeys as string[]}
                onCancel={() => setBatchSSHModalVisible(false)}
                onSuccess={() => {
                    setBatchSSHModalVisible(false);
                    setSelectedRowKeys([]);
                }}
            />

            <BatchVisibilityModal
                open={batchVisibilityModalVisible}
                agentIds={selectedRowKeys as string[]}
                onCancel={() => setBatchVisibilityModalVisible(false)}
                onSuccess={() => {
                    setBatchVisibilityModalVisible(false);
                    setSelectedRowKeys([]);
                }}
            />

            <BatchTrafficStatsModal
                open={batchTrafficModalVisible}
                agentIds={selectedRowKeys as string[]}
                onCancel={() => setBatchTrafficModalVisible(false)}
                onSuccess={() => {
                    setBatchTrafficModalVisible(false);
                    setSelectedRowKeys([]);
                }}
            />
        </div>
    );
};

export default AgentList;
