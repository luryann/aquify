import React, { useState } from 'react';
import { Form, Input, Button, Select, DatePicker, Card, notification } from 'antd';
import { Editor } from '@tinymce/tinymce-react';
import axios from 'axios';

const { Option } = Select;
const { RangePicker } = DatePicker;

const MassEmail: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [emailContent, setEmailContent] = useState('');
  const [selectedTemplate, setSelectedTemplate] = useState('');

  const emailTemplates = [
    { id: '1', name: 'Welcome Email', content: '<p>Welcome to our swim team!</p>' },
    { id: '2', name: 'Payment Reminder', content: '<p>This is a reminder to complete your payment.</p>' },
    { id: '3', name: 'Event Notification', content: '<p>Don\'t forget about the upcoming event.</p>' },
  ];

  const handleTemplateChange = (value: string) => {
    const template = emailTemplates.find((template) => template.id === value);
    setSelectedTemplate(value);
    setEmailContent(template ? template.content : '');
  };

  const handleSendEmail = async (values: any) => {
    setLoading(true);
    try {
      await axios.post('/api/send-email', { ...values, content: emailContent });
      notification.success({
        message: 'Email Sent',
        description: 'The email has been sent successfully.',
      });
      form.resetFields();
      setEmailContent('');
    } catch (error) {
      notification.error({
        message: 'Email Failed',
        description: 'Failed to send the email.',
      });
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ padding: '24px' }}>
      <Card title="Mass Email">
        <Form form={form} layout="vertical" onFinish={handleSendEmail}>
          <Form.Item
            label="Recipient Group"
            name="recipientGroup"
            rules={[{ required: true, message: 'Please select a recipient group!' }]}
          >
            <Select placeholder="Select a group">
              <Option value="all">All Members</Option>
              <Option value="expired">Expired Registrations</Option>
              <Option value="swimmers">Swimmers</Option>
              <Option value="coaches">Coaches</Option>
            </Select>
          </Form.Item>
          <Form.Item
            label="Email Template"
            name="emailTemplate"
            rules={[{ required: true, message: 'Please select an email template!' }]}
          >
            <Select placeholder="Select a template" onChange={handleTemplateChange}>
              {emailTemplates.map((template) => (
                <Option key={template.id} value={template.id}>
                  {template.name}
                </Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item label="Subject" name="subject" rules={[{ required: true, message: 'Please input the subject!' }]}>
            <Input />
          </Form.Item>
          <Form.Item label="Content" required>
            <Editor
              apiKey="YOUR_TINYMCE_API_KEY"
              value={emailContent}
              onEditorChange={(content) => setEmailContent(content)}
              init={{
                height: 300,
                menubar: false,
                plugins: [
                  'advlist autolink lists link image charmap print preview anchor',
                  'searchreplace visualblocks code fullscreen',
                  'insertdatetime media table paste code help wordcount',
                ],
                toolbar:
                  'undo redo | formatselect | bold italic backcolor | \
                  alignleft aligncenter alignright alignjustify | \
                  bullist numlist outdent indent | removeformat | help',
              }}
            />
          </Form.Item>
          <Form.Item label="Schedule Email">
            <RangePicker showTime />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading}>
              Send Email
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
};

export default MassEmail;
