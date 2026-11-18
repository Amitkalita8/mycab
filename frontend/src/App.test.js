import { fireEvent, render, screen } from '@testing-library/react';
import App from './App';

beforeEach(() => localStorage.clear());

test('shows the login screen when logged out', () => {
  render(<App />);
  expect(screen.getByRole('button', { name: /log in/i })).toBeInTheDocument();
  expect(screen.getByLabelText(/phone/i)).toBeInTheDocument();
});

test('driver registration asks for vehicle details', () => {
  render(<App />);
  fireEvent.click(screen.getByRole('button', { name: /i drive/i }));
  fireEvent.click(screen.getByRole('button', { name: /create an account/i }));
  expect(screen.getByLabelText(/number plate/i)).toBeInTheDocument();
});
