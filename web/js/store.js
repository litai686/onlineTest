const KEY = "onlineTest.auth";

export const store = {
  token: "",
  user: null,

  load() {
    try {
      const raw = localStorage.getItem(KEY);
      if (!raw) return;
      const data = JSON.parse(raw);
      if (data && data.token) {
        this.token = data.token;
        this.user = data.user || null;
      }
    } catch {
      localStorage.removeItem(KEY);
    }
  },

  setAuth(token, user) {
    this.token = token;
    this.user = user;
    localStorage.setItem(KEY, JSON.stringify({ token, user }));
  },

  setUser(user) {
    this.user = user;
    localStorage.setItem(KEY, JSON.stringify({ token: this.token, user }));
  },

  logout() {
    this.token = "";
    this.user = null;
    localStorage.removeItem(KEY);
  },

  isTeacher() {
    return this.user?.role === "teacher";
  },

  isStudent() {
    return this.user?.role === "student";
  },
};
