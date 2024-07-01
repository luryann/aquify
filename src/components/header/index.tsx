import React from 'react';
import { Layout, Menu, Dropdown, Avatar, Badge, Typography, Input, Button } from 'antd';
import {
  MenuUnfoldOutlined,
  BellOutlined,
  GlobalOutlined,
  UserOutlined,
  SearchOutlined
} from '@ant-design/icons';
import { useAuth0 } from '@auth0/auth0-react';
import { useNavigate } from 'react-router-dom';
import { useAppContext } from '../../contexts/AppContext';

const { Header: AntHeader } = Layout;
const { Title } = Typography;

const Header: React.FC<{ onToggleSidebar: () => void }> = ({ onToggleSidebar }) => {
  const { logout } = useAuth0();
  const { notifications, clearNotifications } = useAppContext();
  const navigate = useNavigate();

  const notificationMenu = (
    <Menu>
      {notifications.map((notification) => (
        <Menu.Item key={notification.id}>{notification.message}</Menu.Item>
      ))}
      <Menu.Item onClick={clearNotifications}>Clear Notifications</Menu.Item>
    </Menu>
  );

  const userMenu = (
    <Menu>
      <Menu.Item key="0" onClick={() => navigate('/my-account')}>
        My Account
      </Menu.Item>
      <Menu.Item key="1" onClick={() => logout({ returnTo: window.location.origin })}>
        Logout
      </Menu.Item>
    </Menu>
  );

  return (
    <AntHeader className="header" style={{ background: '#fff', padding: 0, height: '64px', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
      <div style={{ display: 'flex', alignItems: 'center', paddingLeft: '24px' }}>
        <MenuUnfoldOutlined style={{ fontSize: '18px', marginRight: '16px' }} onClick={onToggleSidebar} />
        <Title level={4} style={{ margin: 0, marginRight: '16px' }}> aquify 
	<span style={{ fontFamily: 'monospace', fontSize: '14px' }}>  v0.01</span>
        </Title>
        <Input
          prefix={<SearchOutlined />}
          placeholder="Search"
          style={{ width: 200, marginRight: '16px' }}
        />
      </div>
      <div style={{ display: 'flex', alignItems: 'center', paddingRight: '24px' }}>
        <Dropdown overlay={notificationMenu} trigger={['click']}>
          <Button type="text" style={{ marginRight: '0.3rem', position: 'relative' }}>
            <Badge dot={notifications.length > 0} offset={[-2, 2]}>
              <BellOutlined style={{ fontSize: '18px', cursor: 'pointer' }} />
            </Badge>
          </Button>
        </Dropdown>
        <GlobalOutlined
          style={{ fontSize: '18px', marginRight: '16px', cursor: 'pointer' }}
          onClick={() => window.location.href = 'https://dareaquatics.com'}
        />
        <Dropdown overlay={userMenu} trigger={['click']}>
          <Avatar style={{ cursor: 'pointer', fontSize: '18px' }} icon={<UserOutlined />} />
        </Dropdown>
      </div>
    </AntHeader>
  );
};

export default Header;
