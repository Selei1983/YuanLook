import {useState} from 'react';
import {App, Form, Modal, Radio} from 'antd';
import {useMutation, useQueryClient} from '@tanstack/react-query';
import {batchUpdateAgentVisibility} from '@/api/agent.ts';
import {getErrorMessage} from '@/lib/utils';

interface BatchVisibilityModalProps {
    open: boolean;
    agentIds: string[];
    onCancel: () => void;
    onSuccess: () => void;
}

const BatchVisibilityModal = ({open, agentIds, onCancel, onSuccess}: BatchVisibilityModalProps) => {
    const {message: messageApi} = App.useApp();
    const queryClient = useQueryClient();
    const [form] = Form.useForm();
    const [loading, setLoading] = useState(false);

    const updateMutation = useMutation({
        mutationFn: (data: { agentIds: string[]; visibility: string }) =>
            batchUpdateAgentVisibility(data.agentIds, data.visibility),
        onSuccess: () => {
            messageApi.success('批量修改可见性成功');
            queryClient.invalidateQueries({queryKey: ['admin', 'agents']});
            form.resetFields();
            onSuccess();
        },
        onError: (error: unknown) => {
            messageApi.error(getErrorMessage(error, '批量修改可见性失败'));
        },
    });

    const handleOk = async () => {
        try {
            setLoading(true);
            const values = await form.validateFields();
            await updateMutation.mutateAsync({
                agentIds,
                visibility: values.visibility,
            });
        } catch (error) {
            console.error('表单验证失败:', error);
        } finally {
            setLoading(false);
        }
    };

    const handleCancel = () => {
        form.resetFields();
        onCancel();
    };

    return (
        <Modal
            title={`批量修改可见性 (${agentIds.length} 个探针)`}
            open={open}
            onOk={handleOk}
            onCancel={handleCancel}
            confirmLoading={loading}
            destroyOnHidden
            width={500}
        >
            <Form
                form={form}
                layout="vertical"
                initialValues={{
                    visibility: 'private',
                }}
            >
                <Form.Item
                    label="可见性"
                    name="visibility"
                    rules={[{required: true, message: '请选择可见性'}]}
                >
                    <Radio.Group>
                        <Radio value="private">登录可见</Radio>
                    </Radio.Group>
                </Form.Item>
                <div className="text-sm text-gray-500 mt-2">
                    <p>当前为私有模式，仅登录用户可以查看探针信息</p>
                </div>
            </Form>
        </Modal>
    );
};

export default BatchVisibilityModal;
