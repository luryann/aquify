import users from '../mockBackend/users.json';

const mockApi = {
  getUserByEmail: (email: string) => {
    return new Promise((resolve, reject) => {
      const user = users.find(user => user.email === email);
      if (user) {
        resolve(user);
      } else {
        reject('User not found');
      }
    });
  }
};

export default mockApi;
