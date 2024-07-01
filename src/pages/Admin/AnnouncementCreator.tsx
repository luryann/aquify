import React, { useState } from 'react';
import { Form, Input, Button, DatePicker, Switch, Card, notification, Modal } from 'antd';
import { Editor } from '@tinymce/tinymce-react';
import axios from 'axios';

const { RangePicker } = DatePicker;

const AnnouncementCreator: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [previewVisible, setPreviewVisible] = useState(false);
  const [announcementContent, setAnnouncementContent] = useState('');

  const handlePreview = () => {
    setPreviewVisible(true);
  };

  const handlePublish = async (values: any) => {
    setLoading(true);
    try {
      await axios.post('/api/announcements', { ...values, content: announcementContent });
      notification.success({
        message: 'Announcement Published',
        description: 'The announcement has been successfully published.',
      });
      form.resetFields();
      setAnnouncementContent('');
    } catch (error) {
      notification.error({
        message: 'Publication Failed',
        description: 'Failed to publish the announcement.',
      });
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ padding: '24px' }}>
      <Card title="Create Announcement">
        <Form form={form} layout="vertical" onFinish={handlePublish}>
          <Form.Item
            label="Title"
            name="title"
            rules={[{ required: true, message: 'Please input the title!' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item label="Content" required>
            <Editor
              apiKey="YOUR_TINYMCE_API_KEY"
              value={announcementContent}
              onEditorChange={(content) => setAnnouncementContent(content)}
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
          <Form.Item label="Schedule Announcement">
            <RangePicker showTime />
          </Form.Item>
          <Form.Item label="Send Notifications" name="sendNotifications" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading}>
              Publish
            </Button>
            <Button type="default" onClick={handlePreview} style={{ marginLeft: '16px' }}>
              Preview
            </Button>
          </Form.Item>
        </Form>
      </Card>

      <Modal
        title="Preview Announcement"
        visible={previewVisible}
        footer={null}
        onCancel={() => setPreviewVisible(false)}
      >
        <h2>{form.getFieldValue('title')}</h2>
        <div dangerouslySetInnerHTML={{ __html: announcementContent }} />
      </Modal>
    </div>
  );
};

export default AnnouncementCreator;
