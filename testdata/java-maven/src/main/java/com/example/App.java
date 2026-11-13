package com.example;

public class App {
    public static void main(String[] args) {
        Greeter greeter = new Greeter("GoIDEA");
        String message = greeter.greet();
        System.out.println(message);
        int total = 0;
        for (int i = 1; i <= 3; i++) {
            total += i;
        }
        System.out.println("sum=" + total);
    }
}
