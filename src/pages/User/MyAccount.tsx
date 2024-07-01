import React, { useEffect, useState } from 'react';
import { Form, Input, Button, message, Card, Tabs, Tag, Radio, Modal, Table, Select, Row, Col, Statistic, notification } from 'antd';
import { SearchOutlined, ExportOutlined, FileExcelOutlined } from '@ant-design/icons';
import axios from 'axios';

const { TabPane } = Tabs;
const { Option } = Select;

const MyAccount: React.FC = () => {
  const [userData, setUserData] = useState<any>(null);
  const [loading, setLoading] = useState(false);
  const [paymentModalVisible, setPaymentModalVisible] = useState(false);
  const [transactions, setTransactions] = useState([]);
  const [filters, setFilters] = useState({
    dateRange: 'Last 30 Days',
    type: 'All Types',
    status: 'All Statuses',
  });

  const [form] = Form.useForm();
  const [paymentForm] = Form.useForm();

  useEffect(() => {
    fetchUserData();
    fetchTransactions();
  }, [filters]);

  const fetchUserData = async () => {
    setLoading(true);
    try {
      const response = await axios.get('/api/user-data');
      setUserData(response.data);
      form.setFieldsValue(response.data);
    } catch (error) {
      message.error('Failed to fetch user data');
    } finally {
      setLoading(false);
    }
  };

  const fetchTransactions = async () => {
    setLoading(true);
    try {
      const response = await axios.get('/api/billing-summary', { params: filters });
      setTransactions(response.data);
    } catch (error) {
      message.error('Failed to fetch transactions');
    } finally {
      setLoading(false);
    }
  };

  const handleFilterChange = (value: any, key: string) => {
    setFilters({ ...filters, [key]: value });
  };

  const handleSave = async (values: any) => {
    setLoading(true);
    try {
      await axios.put('/api/user-data', values);
      message.success('Account details updated successfully');
      fetchUserData();
      notification.success({
        message: 'Account Update',
        description: 'Your account details have been updated successfully.',
      });
    } catch (error) {
      message.error('Failed to update account details');
    } finally {
      setLoading(false);
    }
  };

  const handleAddPaymentMethod = () => {
    setPaymentModalVisible(true);
  };

  const handlePaymentSubmit = async (values: any) => {
    setLoading(true);
    try {
      await axios.post('/api/add-payment-method', values);
      message.success('Payment method added successfully');
      setPaymentModalVisible(false);
      fetchUserData();
    } catch (error) {
      message.error('Failed to add payment method');
    } finally {
      setLoading(false);
    }
  };

  const columns = [
    {
      title: 'Type',
      dataIndex: 'type',
      key: 'type',
    },
    {
      title: 'Title',
      dataIndex: 'title',
      key: 'title',
      render: (text: string) => <a>{text}</a>,
    },
    {
      title: 'Date',
      dataIndex: 'date',
      key: 'date',
    },
    {
      title: 'Due Date',
      dataIndex: 'dueDate',
      key: 'dueDate',
    },
    {
      title: 'Amount',
      dataIndex: 'amount',
      key: 'amount',
      render: (amount: number) => `$${amount.toFixed(2)}`,
    },
    {
      title: 'Balance',
      dataIndex: 'balance',
      key: 'balance',
      render: (balance: number) => `$${balance.toFixed(2)}`,
    },
    {
      title: 'Actions',
      key: 'actions',
      render: () => <Button icon={<FileExcelOutlined />} />,
    },
  ];

  return (
    <div style={{ padding: '24px' }}>
      <Tabs defaultActiveKey="1">
        <TabPane tab="Account Info" key="1">
          <Card title="Account Information">
            <Form layout="vertical" form={form} onFinish={handleSave}>
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item
                    label="First Name"
                    name="firstName"
                    rules={[{ required: true, message: 'Please input your first name!' }]}
                  >
                    <Input />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item
                    label="Last Name"
                    name="lastName"
                    rules={[{ required: true, message: 'Please input your last name!' }]}
                  >
                    <Input />
                  </Form.Item>
                </Col>
              </Row>
              <Row gutter={16}>
                <Col span={24}>
                  <Form.Item label="Account Login Email">
                    <div>
                      <Tag color={userData?.emailVerified ? 'green' : 'red'}>
                        {userData?.emailVerified ? 'Verified' : 'Unverified'}
                      </Tag>
                      <span style={{ marginLeft: '8px' }}>{userData?.email}</span>
                    </div>
                  </Form.Item>
                </Col>
              </Row>
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item
                    label="Display First Name"
                    name="displayFirstName"
                    rules={[{ required: true, message: 'Please input your display first name!' }]}
                  >
                    <Input />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item
                    label="Display Last Name"
                    name="displayLastName"
                    rules={[{ required: true, message: 'Please input your display last name!' }]}
                  >
                    <Input />
                  </Form.Item>
                </Col>
              </Row>
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item label="Alternate Email 1" name="alternateEmail1">
                    <div>
                      <Tag color={userData?.alternateEmail1Verified ? 'green' : 'red'}>
                        {userData?.alternateEmail1Verified ? 'Verified' : 'Unverified'}
                      </Tag>
                      <Input style={{ marginTop: '8px' }} />
                    </div>
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item label="Alternate Email 2" name="alternateEmail2">
                    <div>
                      <Tag color={userData?.alternateEmail2Verified ? 'green' : 'red'}>
                        {userData?.alternateEmail2Verified ? 'Verified' : 'Unverified'}
                      </Tag>
                      <Input style={{ marginTop: '8px' }} />
                    </div>
                  </Form.Item>
                </Col>
              </Row>
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item label="Mobile/SMS">
                    <div>
                      <Tag color={userData?.mobileVerified ? 'green' : 'red'}>
                        {userData?.mobileVerified ? 'Verified' : 'Unverified'}
                      </Tag>
                      <Input style={{ marginTop: '8px' }} />
                    </div>
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item label="Carrier" name="carrier">
                    <Input />
                  </Form.Item>
                </Col>
              </Row>
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item
                    label="Address"
                    name="address"
                    rules={[{ required: true, message: 'Please input your address!' }]}
                  >
                    <Input />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item
                    label="Address 2"
                    name="address2"
                  >
                    <Input />
                  </Form.Item>
                </Col>
              </Row>
              <Row gutter={16}>
                <Col span={8}>
                  <Form.Item
                    label="City"
                    name="city"
                    rules={[{ required: true, message: 'Please input your city!' }]}
                  >
                    <Input />
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item
                    label="State"
                    name="state"
                    rules={[{ required: true, message: 'Please input your state!' }]}
                  >
                    <Input />
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item
                    label="Zip"
                    name="zip"
                    rules={[{ required: true, message: 'Please input your zip code!' }]}
                  >
                    <Input />
                  </Form.Item>
                </Col>
              </Row>
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item
                    label="Phone"
                    name="phone"
                  >
                    <Input />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item
                    label="Alternate Phone"
                    name="alternatePhone"
                  >
                    <Input />
                  </Form.Item>
                </Col>
              </Row>
              <Form.Item>
                <Button type="primary" htmlType="submit" loading={loading}>
                  Save Changes
                </Button>
              </Form.Item>
            </Form>
          </Card>
        </TabPane>
        <TabPane tab="Billing Summary" key="2">
          <Card title="Billing Summary">
            <Row gutter={16} style={{ marginBottom: '16px' }}>
              <Col span={8}>
                <Statistic title="Unapplied Payment" value={0} prefix="$" />
              </Col>
              <Col span={8}>
                <Statistic title="Balance Due" value={0} prefix="$" />
              </Col>
              <Col span={8}>
                <Statistic title="Overdue" value={0} prefix="$" />
              </Col>
            </Row>
            <Row gutter={16} style={{ marginBottom: '16px' }}>
              <Col span={6}>
                <Input
                  placeholder="Search for items..."
                  prefix={<SearchOutlined />}
                  onChange={(e) => handleFilterChange(e.target.value, 'search')}
                />
              </Col>
              <Col span={6}>
                <Select
                  defaultValue="Last 30 Days"
                  onChange={(value) => handleFilterChange(value, 'dateRange')}
                  style={{ width: '100%' }}
                >
                  <Option value="Last 30 Days">Last 30 Days</Option>
                  <Option value="Last 60 Days">Last 60 Days</Option>
                  <Option value="Last 90 Days">Last 90 Days</Option>
                </Select>
              </Col>
              <Col span={6}>
                <Select
                  defaultValue="All Types"
                  onChange={(value) => handleFilterChange(value, 'type')}
                  style={{ width: '100%' }}
                >
                  <Option value="All Types">All Types</Option>
                  <Option value="Payment">Payment</Option>
                  <Option value="Charge">Charge</Option>
                </Select>
              </Col>
              <Col span={6}>
                <Select
                  defaultValue="All Statuses"
                  onChange={(value) => handleFilterChange(value, 'status')}
                  style={{ width: '100%' }}
                >
                  <Option value="All Statuses">All Statuses</Option>
                  <Option value="Paid">Paid</Option>
                  <Option value="Unpaid">Unpaid</Option>
                  <Option value="Overdue">Overdue</Option>
                </Select>
              </Col>
            </Row>
            <Table
              columns={columns}
              dataSource={transactions}
              loading={loading}
              rowKey="id"
            />
            <Button type="primary" icon={<ExportOutlined />} style={{ marginRight: '16px' }}>
              Export Statement
            </Button>
            <Button icon={<FileExcelOutlined />}>Simulate Billing</Button>
          </Card>
        </TabPane>
        <TabPane tab="Autopay Setup" key="3">
          <Card title="Payment Methods">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
              <Button type="primary" onClick={handleAddPaymentMethod}>Add Credit Card</Button>
              <Button type="default" onClick={handleAddPaymentMethod}>Add Bank Account</Button>
            </div>
            {userData?.paymentMethods?.map((method: any) => (
              <Card key={method.id} style={{ marginBottom: '16px' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                  <div>
                    <p>{method.type === 'credit' ? 'Credit Card' : 'Bank Account'}: **** **** **** {method.last4}</p>
                    <p>Expires: {method.expiry}</p>
                    <p>Name: {method.name}</p>
                  </div>
                  <div>
                    <Radio.Group value={userData?.preferredPaymentMethod} onChange={(e) => console.log('Change preferred payment method:', e.target.value)}>
                      <Radio value={method.id}>Preferred for On-Demand</Radio>
                    </Radio.Group>
                    <Radio.Group value={userData?.autoPaymentMethod} onChange={(e) => console.log('Change auto payment method:', e.target.value)}>
                      <Radio value={method.id}>Auto-Payment</Radio>
                    </Radio.Group>
                  </div>
                </div>
              </Card>
            ))}
          </Card>
        </TabPane>
      </Tabs>

      <Modal
        title="Add Payment Method"
        visible={paymentModalVisible}
        onCancel={() => setPaymentModalVisible(false)}
        footer={null}
      >
        <Form layout="vertical" form={paymentForm} onFinish={handlePaymentSubmit}>
          <Form.Item
            label="Card Number"
            name="cardNumber"
            rules={[{ required: true, message: 'Please input your card number!' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            label="Expiry Date"
            name="expiryDate"
            rules={[{ required: true, message: 'Please input your card expiry date!' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            label="Name on Card"
            name="nameOnCard"
            rules={[{ required: true, message: 'Please input the name on card!' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading}>
              Add Payment Method
            </Button>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default MyAccount;
