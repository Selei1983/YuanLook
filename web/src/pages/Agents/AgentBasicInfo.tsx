import {Descriptions, Space, Spin, Tag} from 'antd';
import {Clock} from 'lucide-react';
import {useQuery} from '@tanstack/react-query';
import dayjs from 'dayjs';
import {getAgentForAdmin, getAgentLatestMetricsForAdmin} from '@/api/agent.ts';

interface AgentBasicInfoProps {
    agentId: string;
}

const AgentBasicInfo = ({agentId}: AgentBasicInfoProps) => {
    const {
        data: agent,
        isLoading: agentLoading,
    } = useQuery({
        queryKey: ['admin', 'agent', agentId],
        queryFn: async () => {
            const response = await getAgentForAdmin(agentId);
            return response.data;
        },
        enabled: !!agentId,
    });

    const {
        data: latestMetrics,
        isLoading: metricsLoading,
    } = useQuery({
        queryKey: ['admin', 'agent', agentId, 'metrics', 'latest'],
        queryFn: async () => {
            const response = await getAgentLatestMetricsForAdmin(agentId);
            return response.data;
        },
        enabled: !!agentId,
    });

    if (agentLoading || metricsLoading) {
        return (
            <div className="text-center py-12">
                <Spin/>
            </div>
        );
    }

    if (!agent) {
        return <div className="text-center py-12 text-gray-500">未找到探针信息</div>;
    }

    const deviceIpInterfaces = (latestMetrics?.networkInterfaces || [])
        .map((netInterface) => ({
            name: netInterface.interface,
            addrs: Array.from(new Set((netInterface.addrs || []).map((addr) => addr.trim()).filter(Boolean))),
        }))
        .filter((netInterface) => netInterface.addrs.length > 0);

    const renderExpireTime = () => {
        if (!agent.expireTime) {
            return <span className="text-gray-400">未设置</span>;
        }

        const expireDate = new Date(agent.expireTime);
        const now = new Date();
        const isExpired = expireDate < now;
        const daysLeft = Math.ceil((expireDate.getTime() - now.getTime()) / (1000 * 60 * 60 * 24));

        return (
            <div className="flex items-center gap-2">
                <span>{expireDate.toLocaleDateString('zh-CN')}</span>
                {isExpired ? (
                    <Tag color="red" variant="filled">已过期</Tag>
                ) : daysLeft <= 7 ? (
                    <Tag color="orange" variant="filled">{daysLeft}天后到期</Tag>
                ) : daysLeft <= 30 ? (
                    <Tag color="gold" variant="filled">{daysLeft}天后到期</Tag>
                ) : null}
            </div>
        );
    };

    return (
        <div>
            <Descriptions column={{xs: 1, sm: 2, lg: 3, xl: 4}} bordered>
                    <Descriptions.Item label="探针名称">{agent.name || '-'}</Descriptions.Item>
                    <Descriptions.Item label="主机名">{agent.hostname || '-'}</Descriptions.Item>
                    <Descriptions.Item label="探针ID">
                        <span className="font-mono text-xs text-gray-600">{agent.id}</span>
                    </Descriptions.Item>

                    <Descriptions.Item label="操作系统">
                        <Tag color="blue">{agent.os}</Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label="系统架构">
                        <Tag>{agent.arch}</Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label="探针版本">{agent.version || '-'}</Descriptions.Item>

                    <Descriptions.Item label="状态">
                        {agent.status === 1 ? (
                            <Tag color="success">在线</Tag>
                        ) : (
                            <Tag color="error">离线</Tag>
                        )}
                    </Descriptions.Item>
                    <Descriptions.Item label="可见性">
                        <Tag color="orange">
                            登录可见
                        </Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label="标签" span={{xs: 1, sm: 2, lg: 3}}>
                        {agent.tags && agent.tags.length > 0 ? (
                            <div className="flex flex-wrap gap-1">
                                {agent.tags.map((tag, index) => (
                                    <Tag key={index} color="blue" variant="filled">{tag}</Tag>
                                ))}
                            </div>
                        ) : (
                            <span className="text-gray-400">无标签</span>
                        )}
                    </Descriptions.Item>

                    <Descriptions.Item label="到期时间" span={{xs: 1, sm: 2, lg: 3}}>
                        {renderExpireTime()}
                    </Descriptions.Item>

                    <Descriptions.Item label="备注" span={{xs: 1, sm: 2, lg: 3}}>
                        {agent.remark ? (
                            <span className="text-gray-700">{agent.remark}</span>
                        ) : (
                            <span className="text-gray-400">未填写</span>
                        )}
                    </Descriptions.Item>

                    <Descriptions.Item label="公网 IPv4">
                        {agent.ipv4 ? (
                            <span className="font-mono text-xs">{agent.ipv4}</span>
                        ) : (
                            <span className="text-gray-400">未获取</span>
                        )}
                    </Descriptions.Item>
                    <Descriptions.Item label="公网 IPv6">
                        {agent.ipv6 ? (
                            <span className="font-mono text-xs">{agent.ipv6}</span>
                        ) : (
                            <span className="text-gray-400">未获取</span>
                        )}
                    </Descriptions.Item>
                    <Descriptions.Item label="通信地址">
                        {agent.ip ? (
                            <span className="font-mono text-xs">{agent.ip}</span>
                        ) : (
                            <span className="text-gray-400">未知</span>
                        )}
                    </Descriptions.Item>

                    <Descriptions.Item label="设备 IP 地址" span={{xs: 1, sm: 2, lg: 3}}>
                        {deviceIpInterfaces.length > 0 ? (
                            <div className="space-y-3">
                                {deviceIpInterfaces.map((netInterface) => (
                                    <div key={netInterface.name}>
                                        <div className="text-xs text-gray-500 mb-1">
                                            {netInterface.name}
                                        </div>
                                        <div className="flex flex-wrap gap-2">
                                            {netInterface.addrs.map((addr) => (
                                                <Tag key={`${netInterface.name}-${addr}`}>
                                                    {addr}
                                                </Tag>
                                            ))}
                                        </div>
                                    </div>
                                ))}
                            </div>
                        ) : (
                            <span className="text-gray-400">暂无数据</span>
                        )}
                    </Descriptions.Item>

                    <Descriptions.Item label="创建时间">
                        {agent.createdAt ? (
                            <Space>
                                <Clock size={14} className="text-gray-400"/>
                                <span>{dayjs(agent.createdAt).format('YYYY-MM-DD HH:mm:ss')}</span>
                            </Space>
                        ) : '-'}
                    </Descriptions.Item>
                    <Descriptions.Item label="更新时间">
                        {agent.updatedAt ? (
                            <Space>
                                <Clock size={14} className="text-gray-400"/>
                                <span>{dayjs(agent.updatedAt).format('YYYY-MM-DD HH:mm:ss')}</span>
                            </Space>
                        ) : '-'}
                    </Descriptions.Item>
                    <Descriptions.Item label="最后活跃时间">
                        {agent.lastSeenAt ? (
                            <Space>
                                <Clock size={14} className="text-gray-400"/>
                                <span>{dayjs(agent.lastSeenAt).format('YYYY-MM-DD HH:mm:ss')}</span>
                            </Space>
                        ) : '-'}
                    </Descriptions.Item>
                </Descriptions>
        </div>
    );
};

export default AgentBasicInfo;
